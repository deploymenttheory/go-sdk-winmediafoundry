package events

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
	"go.uber.org/zap"
)

// WebhookEmitter posts a JSON-encoded BuildEvent to a configurable URL
// whenever HandleEvent is called.
type WebhookEmitter struct {
	url    string
	client *http.Client
	logger *zap.Logger
}

// NewWebhookEmitter creates a WebhookEmitter that POSTs to url.
// If hc is nil, a default client with a 10 s timeout is used.
func NewWebhookEmitter(url string, hc *http.Client, logger *zap.Logger) *WebhookEmitter {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &WebhookEmitter{url: url, client: hc, logger: logger}
}

// webhookPayload is the JSON body sent to the webhook endpoint.
type webhookPayload struct {
	EventType  string        `json:"event_type"`
	Build      catalog.Build `json:"build"`
	OccurredAt time.Time     `json:"occurred_at"`
}

// HandleEvent implements catalog.EventHandler.
func (w *WebhookEmitter) HandleEvent(ctx context.Context, e catalog.BuildEvent) {
	payload := webhookPayload{
		EventType:  string(e.Type),
		Build:      e.Build,
		OccurredAt: time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		w.logger.Error("webhook marshal failed", zap.Error(err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		w.logger.Error("webhook request creation failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		w.logger.Warn("webhook POST failed", zap.String("url", w.url), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		w.logger.Warn("webhook returned non-2xx", zap.String("url", w.url), zap.Int("status", resp.StatusCode))
	}
}
