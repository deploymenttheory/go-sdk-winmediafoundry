package sdk

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
	"github.com/deploymenttheory/go-sdk-uupdump/sdk/transport"
)

// FeedService provides methods for the /v1/feed endpoints.
type FeedService struct {
	t *transport.Transport
}

type listFeedResponse struct {
	Data []catalog.FeedEntry `json:"data"`
	Meta struct {
		Total  int64 `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	} `json:"meta"`
}

// List returns paginated change-feed entries.
func (s *FeedService) List(ctx context.Context, q catalog.FeedQuery) ([]catalog.FeedEntry, int64, error) {
	req := s.t.Request(ctx).SetResult(&listFeedResponse{})

	if !q.Since.IsZero() {
		req = req.SetQueryParam("since", q.Since.UTC().Format(time.RFC3339))
	}
	if q.EventType != "" {
		req = req.SetQueryParam("event_type", q.EventType)
	}
	if q.Limit > 0 {
		req = req.SetQueryParam("limit", fmt.Sprintf("%d", q.Limit))
	}
	if q.Offset > 0 {
		req = req.SetQueryParam("offset", fmt.Sprintf("%d", q.Offset))
	}

	resp, err := req.Get("/v1/feed")
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, 0, fmt.Errorf("feed list: HTTP %d", resp.StatusCode())
	}
	result := resp.Result().(*listFeedResponse)
	return result.Data, result.Meta.Total, nil
}

// SSEEvent is a single Server-Sent Event received from the live feed stream.
type SSEEvent struct {
	Event string
	Data  string
}

// Stream connects to the SSE feed stream and sends events to the returned
// channel until ctx is cancelled or the connection drops.
// The caller is responsible for draining the channel.
func (s *FeedService) Stream(ctx context.Context) (<-chan SSEEvent, error) {
	resp, err := s.t.Request(ctx).
		SetHeader("Accept", "text/event-stream").
		SetDoNotParseResponse(true).
		Get("/v1/feed/stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		resp.RawResponse.Body.Close()
		return nil, fmt.Errorf("feed stream: HTTP %d", resp.StatusCode())
	}

	ch := make(chan SSEEvent, 64)
	go func() {
		defer close(ch)
		defer resp.RawResponse.Body.Close()

		scanner := bufio.NewScanner(resp.RawResponse.Body)
		var ev SSEEvent
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Text()
			if line == "" {
				// Empty line = event dispatch boundary.
				if ev.Data != "" {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
				ev = SSEEvent{}
				continue
			}
			if strings.HasPrefix(line, "event:") {
				ev.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				ev.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
	}()
	return ch, nil
}
