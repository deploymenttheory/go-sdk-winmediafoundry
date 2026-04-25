package middleware

import (
	"net/http"
)

// RequireClientCert rejects requests that do not carry a verified mTLS client
// certificate. This middleware must be placed after TLS termination.
//
// When the server is configured with tls.RequireAndVerifyClientCert the TLS
// handshake itself enforces the certificate requirement; this middleware
// provides an additional application-layer check and can be bypassed for
// endpoints that should be publicly accessible (e.g. /healthz).
//
// When the server is running without TLS (plain HTTP, typically for local
// development), this middleware is a no-op — all /v1/ routes are reachable
// without client certificates.
func RequireClientCert(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Plain HTTP (no TLS) — skip client cert enforcement for dev mode.
		if r.TLS == nil {
			next.ServeHTTP(w, r)
			return
		}
		if len(r.TLS.VerifiedChains) == 0 {
			http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"mTLS client certificate required"}}`,
				http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
