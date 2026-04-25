// validate performs live curl-based validation of the three Windows Update SOAP
// calls implemented in wuproto/soap.
//
// A custom http.RoundTripper intercepts every request the real SOAPClient
// makes, saves the SOAP envelope to a temp file, then shells out to curl.
// curl handles the actual HTTPS transport and prints request/response headers.
// The response body is fed back to the SOAPClient so XML parsing is exercised.
//
// Three SOAP calls are validated in sequence:
//
//  1. GetCookie            → https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx
//  2. SyncUpdates          → https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx
//  3. GetExtendedUpdateInfo2 → https://fe3cr.delivery.mp.microsoft.com/ClientWebService/client.asmx/secured
//
// Usage:
//
//	go run ./cmd/validate
//	go run ./cmd/validate --arch arm64 --ring Dev
//	go run ./cmd/validate --step getcookie   # only run step 1
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/wuproto"
	"github.com/deploymenttheory/go-sdk-uupdump/wuproto/soap"
	"go.uber.org/zap"
)

func main() {
	arch := flag.String("arch", "amd64", "architecture (amd64, arm64, x86)")
	ring := flag.String("ring", "Retail", "ring (Canary, Dev, Beta, ReleasePreview, Retail)")
	step := flag.String("step", "all", "step: getcookie | syncupdates | fileurl | all")
	flag.Parse()

	logger, _ := zap.NewDevelopment()
	defer logger.Sync() //nolint:errcheck

	// The WU endpoint certs are signed by "Microsoft Update Secure Server CA 2.1",
	// an intermediate CA not present in the default macOS trust store.
	// Build a combined bundle at startup so curl can verify the chain.
	caBundle, err := buildWUCABundle()
	if err != nil {
		fatalf("build CA bundle: %v", err)
	}
	defer os.Remove(caBundle)

	ct := &curlTransport{logger: logger, caBundle: caBundle}

	// soap.New generates a device token and acquires a WU cookie immediately.
	// That first GetCookie call goes through curlTransport.
	banner("STEP 1 — GetCookie")
	fmt.Println("  Acquiring WU session cookie via soap.New …")

	client, err := soap.New(
		logger,
		soap.WithHTTPClient(&http.Client{
			Transport: ct,
			Timeout:   90 * time.Second,
		}),
	)
	if err != nil {
		fatalf("soap.New / GetCookie: %v", err)
	}
	fmt.Printf("  ✓ GetCookie succeeded (%d curl call(s))\n", ct.calls)

	if *step == "getcookie" {
		return
	}

	// ── Step 2: SyncUpdates ─────────────────────────────────────────────────
	banner("STEP 2 — SyncUpdates")
	fmt.Printf("  Querying arch=%s ring=%s …\n", *arch, *ring)
	ct.reset()

	results, err := client.FetchUpdates(context.Background(), wuproto.FetchRequest{
		Arch:   wuproto.Arch(*arch),
		Ring:   wuproto.Ring(*ring),
		Flight: wuproto.FlightActive,
	})
	if err != nil {
		fatalf("FetchUpdates / SyncUpdates: %v", err)
	}
	fmt.Printf("  ✓ SyncUpdates succeeded (%d curl call(s)) — %d update(s) returned\n", ct.calls, len(results))
	for i, r := range results {
		if i >= 5 {
			fmt.Printf("    … and %d more\n", len(results)-5)
			break
		}
		fmt.Printf("    [%d] UpdateID=%-12s Rev=%-4d Build=%-15s Arch=%s Files=%d\n",
			i+1, r.UpdateID, r.Revision, r.Build, r.Arch, len(r.Files))
	}

	if *step == "syncupdates" || len(results) == 0 {
		if len(results) == 0 {
			fmt.Fprintln(os.Stderr, "  ⚠  no updates returned — skipping GetExtendedUpdateInfo2")
		}
		return
	}

	// ── Step 3: GetExtendedUpdateInfo2 ──────────────────────────────────────
	// Prefer a Windows OS update (build "10.0.*") for EUI2 validation, since
	// component packages (MSRT, SedimentPack) often return empty FileLocations.
	first := results[0]
	for _, r := range results {
		if strings.HasPrefix(r.Build, "10.0.") {
			first = r
			break
		}
	}
	banner(fmt.Sprintf("STEP 3 — GetExtendedUpdateInfo2\n  UpdateID=%s  Rev=%d", first.UpdateID, first.Revision))
	ct.reset()

	urls, err := client.GetFileURLs(context.Background(), wuproto.FileURLRequest{
		UpdateID: first.UpdateID,
		Revision: first.Revision,
	})
	if err != nil {
		fatalf("GetFileURLs / GetExtendedUpdateInfo2: %v", err)
	}
	fmt.Printf("  ✓ GetExtendedUpdateInfo2 succeeded (%d curl call(s)) — %d file URL(s) returned\n", ct.calls, len(urls))
	for i, u := range urls {
		if i >= 5 {
			fmt.Printf("    … and %d more\n", len(urls)-5)
			break
		}
		exp := ""
		if !u.ExpiresAt.IsZero() {
			exp = "  expires=" + u.ExpiresAt.Format(time.RFC3339)
		}
		fmt.Printf("    [%d] %-60s  %d bytes%s\n", i+1, u.Name, u.SizeBytes, exp)
	}

	banner("ALL STEPS PASSED")
}

// ─── curlTransport ────────────────────────────────────────────────────────────

// curlTransport implements http.RoundTripper using curl as the transport.
// For each request it:
//  1. Writes the request body to a temp file.
//  2. Runs curl: verbose headers → stderr (visible to terminal), response
//     body → temp file, HTTP status → captured from --write-out.
//  3. Returns a synthesised *http.Response to the caller.
type curlTransport struct {
	logger   *zap.Logger
	caBundle string // path to combined CA bundle file
	calls    int
}

func (t *curlTransport) reset() { t.calls = 0 }

func (t *curlTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	step := t.calls

	// ── Save request body to temp file ─────────────────────────────────────
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	reqFile, err := tempFile("wu-req-*.xml", bodyBytes)
	if err != nil {
		return nil, err
	}
	defer os.Remove(reqFile)

	// ── Temp file for response body ────────────────────────────────────────
	respFile, err := tempFile("wu-resp-*.xml", nil)
	if err != nil {
		return nil, err
	}
	// Don't auto-remove resp files — caller can inspect them if needed.
	// defer os.Remove(respFile)

	// ── Build curl args ────────────────────────────────────────────────────
	args := []string{
		"--silent",     // suppress progress meter
		"--show-error", // but do print errors
		"--verbose",    // print request + response headers to stderr
		"--compressed",
		"--max-time", "60",
		// The Microsoft Update endpoints use an intermediate CA not in the
		// default macOS trust store. Use the bundle built by cmd/validate.
		"--cacert", t.caBundle,
		"--request", req.Method,
		"--url", req.URL.String(),
		"--header", "Content-Type: " + req.Header.Get("Content-Type"),
		"--header", "User-Agent: " + req.Header.Get("User-Agent"),
		"--data-binary", "@" + reqFile,
		"--output", respFile,          // response body → file
		"--write-out", "%{http_code}", // HTTP status → stdout (captured)
	}

	if sa := req.Header.Get("SOAPAction"); sa != "" {
		args = append(args, "--header", "SOAPAction: "+sa)
	}

	// ── Print the command so the user can re-run it ────────────────────────
	fmt.Printf("\n  [call %d] curl %s\n\n", step, strings.Join(args, " "))

	// ── Run curl ───────────────────────────────────────────────────────────
	// stdout  → status code (captured)
	// stderr  → verbose headers (printed directly to terminal)
	var statusBuf bytes.Buffer
	cmd := exec.Command("curl", args...)
	cmd.Stdout = &statusBuf
	cmd.Stderr = os.Stderr
	if runErr := cmd.Run(); runErr != nil {
		return nil, fmt.Errorf("curl (call %d) failed: %w", step, runErr)
	}

	// ── Parse HTTP status ──────────────────────────────────────────────────
	statusCode := http.StatusOK
	if sc, err := strconv.Atoi(strings.TrimSpace(statusBuf.String())); err == nil {
		statusCode = sc
	}
	fmt.Printf("\n  [call %d] HTTP %d\n", step, statusCode)

	// ── Read response body ────────────────────────────────────────────────
	respBody, err := os.ReadFile(respFile)
	if err != nil {
		return nil, fmt.Errorf("read curl response body: %w", err)
	}

	resp := &http.Response{
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode: statusCode,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	return resp, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func tempFile(pattern string, content []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file %s: %w", pattern, err)
	}
	defer f.Close()
	if len(content) > 0 {
		if _, err := f.Write(content); err != nil {
			os.Remove(f.Name())
			return "", fmt.Errorf("write temp file: %w", err)
		}
	}
	return f.Name(), nil
}

func banner(title string) {
	line := strings.Repeat("─", 72)
	fmt.Printf("\n%s\n  %s\n%s\n", line, title, line)
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "\nFAIL: "+format+"\n", a...)
	os.Exit(1)
}

// buildWUCABundle downloads both Microsoft Update intermediate CA certificates
// (RSA and ECC variants) and concatenates them with the system CA bundle into
// a temp PEM file suitable for curl's --cacert flag.
//
// fe3.delivery.mp.microsoft.com  → "Microsoft Update Secure Server CA 2.1" (RSA)
// fe3cr.delivery.mp.microsoft.com → "Microsoft ECC Update Secure Server CA 2.1" (ECC)
//
// Neither intermediate is included in the macOS trust store at /etc/ssl/cert.pem.
// Without them curl exits 60 (SSL certificate verify failed).
func buildWUCABundle() (string, error) {
	const systemCAPath = "/etc/ssl/cert.pem"

	msftCAs := []struct {
		name string
		url  string
	}{
		{
			name: "Microsoft Update Secure Server CA 2.1 (RSA)",
			url:  "http://www.microsoft.com/pkiops/certs/Microsoft%20Update%20Secure%20Server%20CA%202.1.crt",
		},
		{
			name: "Microsoft ECC Update Secure Server CA 2.1 (ECC)",
			url:  "http://www.microsoft.com/pkiops/certs/Microsoft%20ECC%20Update%20Secure%20Server%20CA%202.1.crt",
		},
	}

	// Read system CA bundle.
	systemCA, _ := os.ReadFile(systemCAPath)

	// Write combined bundle.
	f, err := os.CreateTemp("", "wu-ca-bundle-*.pem")
	if err != nil {
		return "", fmt.Errorf("create CA bundle file: %w", err)
	}
	defer f.Close()
	if len(systemCA) > 0 {
		_, _ = f.Write(systemCA)
	}

	for _, ca := range msftCAs {
		fmt.Printf("  Fetching %s …\n", ca.name)
		resp, err := http.Get(ca.url) //nolint:noctx,gosec
		if err != nil {
			return "", fmt.Errorf("download %s: %w", ca.name, err)
		}
		derBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read %s: %w", ca.name, readErr)
		}
		pemBlock := "-----BEGIN CERTIFICATE-----\n" +
			base64Wrap(derBytes) +
			"-----END CERTIFICATE-----\n"
		_, _ = f.WriteString(pemBlock)
	}

	fmt.Printf("  CA bundle written to %s\n", f.Name())
	return f.Name(), nil
}

// base64Wrap encodes bytes as base64 with 64-char line wrapping (standard PEM format).
func base64Wrap(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for i := 0; i < len(enc); i += 64 {
		end := i + 64
		if end > len(enc) {
			end = len(enc)
		}
		sb.WriteString(enc[i:end])
		sb.WriteByte('\n')
	}
	return sb.String()
}
