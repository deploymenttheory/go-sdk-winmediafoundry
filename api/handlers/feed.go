package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/api/response"
	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
)

// FeedHandler handles the change-feed endpoints (paginated + SSE).
type FeedHandler struct {
	svc *winupdate.Service
	// subscribeToEmitter allows the SSE endpoint to register for live events.
	subscribe func(catalog.EventHandler) func()
}

// NewFeedHandler creates a FeedHandler.
// subscribe is the EventEmitter.Subscribe method (for live SSE streaming).
func NewFeedHandler(svc *winupdate.Service, subscribe func(catalog.EventHandler) func()) *FeedHandler {
	return &FeedHandler{svc: svc, subscribe: subscribe}
}

// List handles GET /v1/feed — paginated history.
//
// Query parameters:
//
//	since=<RFC3339>       filter entries after this timestamp
//	event_type=<string>   filter by event type
//	limit=<int>
//	offset=<int>
func (h *FeedHandler) List(w http.ResponseWriter, r *http.Request) {
	q := catalog.FeedQuery{
		EventType: r.URL.Query().Get("event_type"),
		Limit:     parseIntParam(r, "limit", 100),
		Offset:    parseIntParam(r, "offset", 0),
	}

	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			response.BadRequest(w, "since must be RFC3339: "+err.Error())
			return
		}
		q.Since = t
	}

	entries, total, err := h.svc.GetFeed(r.Context(), q)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if entries == nil {
		entries = []catalog.FeedEntry{}
	}
	response.OKPaged(w, entries, total, q.Limit, q.Offset)
}

// Stream handles GET /v1/feed/stream — Server-Sent Events live stream.
func (h *FeedHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalError(w, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Channel for events dispatched from the bus.
	events := make(chan catalog.BuildEvent, 16)
	unsubscribe := h.subscribe(catalog.EventHandlerFunc(func(_ context.Context, e catalog.BuildEvent) {
		select {
		case events <- e:
		default: // drop if buffer full rather than block the bus
		}
	}))
	defer unsubscribe()

	// Send a keep-alive comment every 15 s so proxies don't close the connection.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-events:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", string(e.Type), data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}
