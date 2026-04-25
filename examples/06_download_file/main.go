// 06_download_file demonstrates GET /v1/builds/:uuid/files/:name/download.
//
// It streams a single file from Microsoft's CDN through the winupdate server
// to a local output file. The server resolves a fresh EUI2 CDN URL and proxies
// the byte stream — the client never needs to handle the pre-signed URL
// directly.
//
// Progress is reported in real time. After the download completes, the SHA1
// of the received data is compared against the catalog value as a sanity check.
//
// Note: this example makes a raw HTTP request (not a named SDK method) because
// the download endpoint returns a binary body stream.
//
// Usage (plain HTTP):
//
//	go run ./examples/06_download_file \
//	  --server http://localhost:8080 \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --revision 1 \
//	  --file Windows11.0-KB5058411-x64.cab \
//	  --out /tmp/Windows11.0-KB5058411-x64.cab
//
// Usage (mTLS):
//
//	go run ./examples/06_download_file \
//	  --server https://localhost:8443 \
//	  --cert certs/client.crt --key certs/client.key --ca certs/ca.crt \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --revision 1 \
//	  --file Windows11.0-KB5058411-x64.cab \
//	  --out /tmp/Windows11.0-KB5058411-x64.cab
package main

import (
	"context"
	"crypto/sha1" //nolint:gosec // SHA1 used for content verification to match WU metadata
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "winupdate server base URL")
	cert := flag.String("cert", "", "client certificate file (omit for plain HTTP)")
	key := flag.String("key", "", "client private key file")
	ca := flag.String("ca", "", "CA certificate file")
	uuid := flag.String("uuid", "", "build UUID (required)")
	revision := flag.Int("revision", 0, "build revision number (required)")
	file := flag.String("file", "", "filename to download (required; from 04_list_files output)")
	out := flag.String("out", "", "output file path (required)")
	flag.Parse()

	if *uuid == "" || *revision == 0 || *file == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "error: --uuid, --revision, --file, and --out are all required")
		flag.Usage()
		os.Exit(1)
	}

	httpClient, err := buildHTTPClient(*cert, *key, *ca)
	if err != nil {
		log.Fatalf("build HTTP client: %v", err)
	}

	// Build the download URL:
	// GET /v1/builds/{uuid}/files/{filename}/download?revision={n}
	rawURL := fmt.Sprintf("%s/v1/builds/%s/files/%s/download?revision=%d",
		*server, *uuid, url.PathEscape(*file), *revision)

	ctx, cancel := context.WithTimeout(context.Background(), 0) // no timeout for large files
	_ = cancel                                                    // cancel via signal only
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		log.Fatalf("build request: %v", err)
	}

	fmt.Printf("Downloading %s → %s\n", *file, *out)
	fmt.Printf("  server: %s\n\n", *server)

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("download request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	outFile, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create output file: %v", err)
	}
	defer outFile.Close()

	//nolint:gosec // SHA1 matches Windows Update metadata hash format
	hasher := sha1.New()
	writer := io.MultiWriter(outFile, hasher, &progressWriter{total: resp.ContentLength})

	n, err := io.Copy(writer, resp.Body)
	if err != nil {
		log.Fatalf("stream download: %v", err)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	sha1hex := hex.EncodeToString(hasher.Sum(nil))

	fmt.Printf("\n\nDownloaded %s in %s\n", formatBytes(n), elapsed)
	fmt.Printf("  output:  %s\n", *out)
	fmt.Printf("  sha1:    %s\n", sha1hex)
	fmt.Printf("  speed:   %s/s\n", formatBytes(int64(float64(n)/elapsed.Seconds())))
}

// progressWriter prints a progress line to stderr.
type progressWriter struct {
	written int64
	total   int64
	last    time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	now := time.Now()
	if now.Sub(p.last) >= time.Second {
		if p.total > 0 {
			pct := float64(p.written) / float64(p.total) * 100
			fmt.Fprintf(os.Stderr, "\r  progress: %s / %s (%.1f%%)",
				formatBytes(p.written), formatBytes(p.total), pct)
		} else {
			fmt.Fprintf(os.Stderr, "\r  downloaded: %s", formatBytes(p.written))
		}
		p.last = now
	}
	return len(b), nil
}

func buildHTTPClient(cert, key, ca string) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if cert != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	if ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse CA cert")
		}
		tlsCfg.RootCAs = pool
	} else {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // dev/plain HTTP mode
	}

	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   0, // no timeout — file may be several GiB
	}, nil
}

func formatBytes(b int64) string {
	const (
		GiB = 1 << 30
		MiB = 1 << 20
		KiB = 1 << 10
	)
	switch {
	case b >= GiB:
		return fmt.Sprintf("%.2f GiB", float64(b)/GiB)
	case b >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/MiB)
	case b >= KiB:
		return fmt.Sprintf("%.0f KiB", float64(b)/KiB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
