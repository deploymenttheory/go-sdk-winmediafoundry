package soap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"resty.dev/v3"
)

const (
	// Endpoint for cookie acquisition and SyncUpdates.
	clientEndpoint = "https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx"

	// Endpoint for GetExtendedUpdateInfo2 (secured).
	clientSecuredEndpoint = "https://fe3cr.delivery.mp.microsoft.com/ClientWebService/client.asmx/secured"

	// userAgent matches the Windows Update Agent used by the UPP implementation.
	userAgent = "Windows-Update-Agent/10.0.10011.16384 Client-Protocol/2.50"

	// cookieTTL controls how long a cached WU session cookie is considered fresh.
	cookieTTL = 14 * time.Minute
)

// cachedCookie holds a WU session cookie with its expiry.
type cachedCookie struct {
	encryptedData string
	expiration    string // W3C timestamp from the server
	expiresAt     time.Time
}

// isValid returns true if the cookie is present and has not expired (with a
// small safety margin).
func (c *cachedCookie) isValid() bool {
	return c != nil && c.encryptedData != "" && time.Now().Before(c.expiresAt)
}

// cookieManager handles device token generation, WU cookie acquisition, and
// in-process caching. It is safe for concurrent use.
type cookieManager struct {
	mu          sync.RWMutex
	cookie      *cachedCookie
	deviceToken string        // generated once, reused
	rc          *resty.Client // shared resty client (SOAP defaults pre-applied)
	logger      *zap.Logger
}

func newCookieManager(rc *resty.Client, logger *zap.Logger) (*cookieManager, error) {
	token, err := generateDeviceToken()
	if err != nil {
		return nil, fmt.Errorf("generate device token: %w", err)
	}
	return &cookieManager{
		deviceToken: token,
		rc:          rc,
		logger:      logger,
	}, nil
}

// get returns a valid cookie, refreshing it if necessary.
func (m *cookieManager) get(ctx context.Context) (*cachedCookie, error) {
	m.mu.RLock()
	if m.cookie.isValid() {
		c := m.cookie
		m.mu.RUnlock()
		return c, nil
	}
	m.mu.RUnlock()

	// Upgrade to write lock for refresh.
	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if m.cookie.isValid() {
		return m.cookie, nil
	}

	m.logger.Debug("acquiring Windows Update session cookie")
	c, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}
	m.cookie = c
	return c, nil
}

// acquire performs the GetCookie SOAP call and returns a fresh cachedCookie.
func (m *cookieManager) acquire(ctx context.Context) (*cachedCookie, error) {
	now := time.Now().UTC()
	body := buildGetCookieEnvelope(now, m.deviceToken)

	resp, err := m.post(ctx, clientEndpoint,
		"http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetCookie",
		body)
	if err != nil {
		return nil, fmt.Errorf("GetCookie SOAP call: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("GetCookie returned HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var envelope getCookieEnvelope
	if err := xml.Unmarshal(resp.Bytes(), &envelope); err != nil {
		return nil, fmt.Errorf("parse GetCookie response: %w", err)
	}

	result := envelope.Body.Result
	if result.EncryptedData == "" {
		return nil, fmt.Errorf("GetCookie response contained no EncryptedData")
	}

	expiresAt := now.Add(cookieTTL)
	return &cachedCookie{
		encryptedData: result.EncryptedData,
		expiration:    result.Expiration,
		expiresAt:     expiresAt,
	}, nil
}

// post performs a SOAP HTTP POST. Content-Type and User-Agent are inherited
// from the resty client; only the per-call SOAPAction header is set here.
func (m *cookieManager) post(ctx context.Context, url, action, body string) (*resty.Response, error) {
	req := m.rc.R().SetContext(ctx).SetBody(body)
	if action != "" {
		req.SetHeader("SOAPAction", action)
	}
	return req.Post(url)
}

// invalidate clears the cached cookie so the next call triggers a refresh.
func (m *cookieManager) invalidate() {
	m.mu.Lock()
	m.cookie = nil
	m.mu.Unlock()
}

// ─── Device token generation ───────────────────────────────────────────────

// Fixed header and footer hex strings from the PHP source (uupDevice()).
const (
	deviceTokenHeader = "13003002c377040014d5bcac7a66de0d50beddf9bba16c87edb9e019898000"
	deviceTokenFooter = "b401"
)

// generateDeviceToken produces a base64-encoded device ticket formatted as
// the Windows Update SOAP service expects.
//
// Algorithm (from shared/utils.php uupDevice()):
//  1. Concatenate header (29 bytes) + 527 random bytes + footer (2 bytes) in hex.
//  2. Decode hex → binary (558 bytes total).
//  3. base64-encode the binary.
//  4. Build the ticket string: "t=<b64>&p=".
//  5. Null-pad each character of the ticket: chunk_split(ticket, 1, "\0").
//  6. base64-encode the null-padded string — this is the device token.
func generateDeviceToken() (string, error) {
	// Step 1 & 2: build 527 random bytes and assemble hex string.
	random := make([]byte, 527)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate random device token bytes: %w", err)
	}
	hexStr := deviceTokenHeader + hex.EncodeToString(random) + deviceTokenFooter

	// Step 3: hex → binary → base64.
	binData, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("decode device token hex: %w", err)
	}
	b64Data := base64.StdEncoding.EncodeToString(binData)

	// Step 4: ticket string.
	ticket := "t=" + b64Data + "&p="

	// Step 5: null-pad each byte of the ticket (chunk_split equivalent).
	nullPadded := make([]byte, len(ticket)*2)
	for i, b := range []byte(ticket) {
		nullPadded[i*2] = b
		nullPadded[i*2+1] = 0x00
	}

	// Step 6: base64-encode the null-padded string.
	return base64.StdEncoding.EncodeToString(nullPadded), nil
}
