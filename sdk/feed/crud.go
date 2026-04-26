// Package feed provides methods for the /v1/feed API endpoints.
package feed

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
)

// Feed provides methods for the /v1/feed endpoints.
type Feed struct {
	t *transport.Transport
}

// New returns a new Feed service backed by the given transport.
func New(t *transport.Transport) *Feed {
	return &Feed{t: t}
}

// List returns paginated change-feed entries.
func (f *Feed) List(ctx context.Context, q catalog.FeedQuery) ([]catalog.FeedEntry, int64, error) {
	req := f.t.Request(ctx).SetResult(&listResponse{})

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

	resp, err := req.Get(constants.EndpointFeed)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, 0, fmt.Errorf("feed list: HTTP %d", resp.StatusCode())
	}
	result := resp.Result().(*listResponse)
	return result.Data, result.Meta.Total, nil
}

// Stream connects to the SSE feed stream and sends events to the returned
// channel until ctx is cancelled or the connection drops.
// The caller is responsible for draining the channel.
func (f *Feed) Stream(ctx context.Context) (<-chan Event, error) {
	resp, err := f.t.Request(ctx).
		SetHeader("Accept", constants.ContentTypeSSE).
		SetDoNotParseResponse(true).
		Get(constants.EndpointFeedStream)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		resp.RawResponse.Body.Close()
		return nil, fmt.Errorf("feed stream: HTTP %d", resp.StatusCode())
	}

	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		defer resp.RawResponse.Body.Close()

		scanner := bufio.NewScanner(resp.RawResponse.Body)
		var ev Event
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
				ev = Event{}
				continue
			}
			if v, ok := strings.CutPrefix(line, "event:"); ok {
				ev.Event = strings.TrimSpace(v)
			} else if v, ok := strings.CutPrefix(line, "data:"); ok {
				ev.Data = strings.TrimSpace(v)
			}
		}
	}()
	return ch, nil
}
