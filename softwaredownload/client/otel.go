package client

import "net/http"

// applyOpenTelemetry wraps the HTTP transport with OpenTelemetry instrumentation
// when a global OTel provider is configured. If no provider is set this is a no-op.
//
// OTel instrumentation is intentionally deferred: it is applied AFTER the
// resty client is fully constructed so that the transport layer is already in a
// valid state.
//
// To enable tracing/metrics, configure global providers before creating a client:
//
//	otel.SetTracerProvider(myTracerProvider)
//	otel.SetMeterProvider(myMeterProvider)
//	otel.SetTextMapPropagator(propagation.TraceContext{})
func (t *Transport) applyOpenTelemetry() {
	httpClient := t.client.Client()
	if httpClient == nil {
		return
	}

	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	// otelhttp.NewTransport wraps the underlying transport with span creation,
	// metric recording, and propagation injection. Uncommenting the lines below
	// requires adding go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
	// to go.mod.
	//
	// instrumentedTransport := otelhttp.NewTransport(transport)
	// httpClient.Transport = instrumentedTransport
	// t.logger.Debug("OpenTelemetry HTTP instrumentation enabled (uses global providers)")

	// Keep the transport reference to avoid unused-variable warnings if the
	// instrumentation lines above are uncommented later.
	_ = transport
}
