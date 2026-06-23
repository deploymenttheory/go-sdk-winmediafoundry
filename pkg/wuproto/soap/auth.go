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

// CookieManager handles device token generation, WU cookie acquisition, and
// in-process caching. It is safe for concurrent use.
//
// The Transport holds a *CookieManager and delegates AcquireWUCookie /
// InvalidateWUCookie to it. Services call AcquireWUCookie to embed the cookie
// values in SOAP request bodies before using NewRequest to send them.
type CookieManager struct {
	mu          sync.RWMutex
	cookie      *cachedCookie
	DeviceToken string        // generated once at construction, reused across calls
	rc          *resty.Client // shared with the Transport's resty client
	logger      *zap.Logger
}

// NewCookieManager creates a CookieManager, generating a device token eagerly
// so that any CSPRNG failures are surfaced at construction time.
func NewCookieManager(rc *resty.Client, logger *zap.Logger) (*CookieManager, error) {
	token, err := generateDeviceToken()
	if err != nil {
		return nil, fmt.Errorf("generate device token: %w", err)
	}
	return &CookieManager{
		DeviceToken: token,
		rc:          rc,
		logger:      logger,
	}, nil
}

// Get returns a valid cookie, refreshing it if necessary.
// Returns the flat encryptedData, expiration, and DeviceToken values needed to
// construct SOAP request bodies.
func (m *CookieManager) Get(ctx context.Context) (encryptedData, expiration, deviceToken string, err error) {
	m.mu.RLock()
	if m.cookie.isValid() {
		c := m.cookie
		m.mu.RUnlock()
		return c.encryptedData, c.expiration, m.DeviceToken, nil
	}
	m.mu.RUnlock()

	// Upgrade to write lock for refresh.
	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if m.cookie.isValid() {
		return m.cookie.encryptedData, m.cookie.expiration, m.DeviceToken, nil
	}

	m.logger.Debug("acquiring Windows Update session cookie")
	c, err := m.acquire(ctx)
	if err != nil {
		return "", "", "", err
	}
	m.cookie = c
	return c.encryptedData, c.expiration, m.DeviceToken, nil
}

// Invalidate clears the cached cookie so the next Get call triggers a refresh.
// Call this when a SOAP response indicates a stale or expired cookie
// (see IsCookieError).
func (m *CookieManager) Invalidate() {
	m.mu.Lock()
	m.cookie = nil
	m.mu.Unlock()
}

// acquire performs the GetCookie SOAP call and returns a fresh cachedCookie.
func (m *CookieManager) acquire(ctx context.Context) (*cachedCookie, error) {
	now := time.Now().UTC()
	body := buildGetCookieEnvelope(now, m.DeviceToken)

	resp, err := m.post(ctx, ClientEndpoint, GetCookieAction, body)
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

// post performs a SOAP HTTP POST. Content-Type and User-Agent are set per-request.
func (m *CookieManager) post(ctx context.Context, url, action, body string) (*resty.Response, error) {
	req := m.rc.R().
		SetContext(ctx).
		SetBody(body).
		SetHeader("Content-Type", ContentType).
		SetHeader("User-Agent", UserAgent)
	if action != "" {
		req.SetHeader("SOAPAction", action)
	}
	return req.Post(url)
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
	random := make([]byte, 527)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate random device token bytes: %w", err)
	}
	hexStr := deviceTokenHeader + hex.EncodeToString(random) + deviceTokenFooter

	binData, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("decode device token hex: %w", err)
	}
	b64Data := base64.StdEncoding.EncodeToString(binData)

	ticket := "t=" + b64Data + "&p="

	nullPadded := make([]byte, len(ticket)*2)
	for i, b := range []byte(ticket) {
		nullPadded[i*2] = b
		nullPadded[i*2+1] = 0x00
	}

	return base64.StdEncoding.EncodeToString(nullPadded), nil
}
