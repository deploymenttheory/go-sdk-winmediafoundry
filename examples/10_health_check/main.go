// 10_health_check demonstrates GET /healthz and GET /readyz.
//
// Both endpoints are publicly accessible — they do not require a client
// certificate. Use them for monitoring, container liveness/readiness probes,
// or as a pre-flight check before running other examples.
//
//   /healthz — always returns 200 {"status":"ok"} if the server process is up
//   /readyz  — returns 200 if the database is reachable; 503 otherwise
//
// Exit code 0 means both probes passed. Exit code 1 means at least one failed.
//
// Usage:
//
//	go run ./examples/10_health_check --server http://localhost:8080
//	go run ./examples/10_health_check --server https://localhost:8443
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "winupdate server base URL")
	flag.Parse()

	// Health endpoints are unauthenticated — plain HTTP client is fine.
	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	liveness := probe(ctx, client, *server+"/healthz")
	readiness := probe(ctx, client, *server+"/readyz")

	fmt.Printf("healthz  %s\n", statusLine(liveness))
	fmt.Printf("readyz   %s\n", statusLine(readiness))

	if liveness.err != nil || readiness.err != nil || liveness.code != 200 || readiness.code != 200 {
		os.Exit(1)
	}
}

type probeResult struct {
	code int
	body map[string]any
	err  error
}

func probe(ctx context.Context, c *http.Client, url string) probeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return probeResult{err: err}
	}
	resp, err := c.Do(req)
	if err != nil {
		return probeResult{err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return probeResult{code: resp.StatusCode, body: body}
}

func statusLine(r probeResult) string {
	if r.err != nil {
		return fmt.Sprintf("FAIL  error: %v", r.err)
	}
	if r.code == 200 {
		status, _ := r.body["status"].(string)
		return fmt.Sprintf("OK    %d  %s", r.code, status)
	}
	return fmt.Sprintf("FAIL  %d  %v", r.code, r.body)
}
