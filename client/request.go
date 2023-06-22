package client

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/chromedp/chromedp"
)

// Request is a small wrapper around *http.Request that contains Metadata and Rendering option
type Request struct {
	*http.Request

	// Meta contains arbitrary data.
	// Use this Meta map to store contextual data between your requests
	Meta map[string]interface{}

	// If true, requests will be synchronized
	Synchronized bool

	// If true request will be opened in Chrome and
	// fully rendered HTML DOM response will returned as response
	Rendered bool

	// Optional response body encoding. Leave empty for automatic detection.
	// If you're having issues with auto detection, set this.
	Encoding string

	// Set this true to cancel requests. Should be used on middlewares.
	Cancelled bool

	// Chrome actions to be run if the request is Rendered
	Actions []chromedp.Action

	retryCounter int32
}

// Cancel request
func (r *Request) Cancel() {
	r.Cancelled = true
}

func (r *Request) RetryCountInc() int {
	return int(atomic.AddInt32(&r.retryCounter, 1))
}

func (r *Request) RetryCount() int {
	return int(atomic.LoadInt32(&r.retryCounter))
}

// NewRequest returns a new Request given a method, URL, and optional body.
func NewRequest(ctx context.Context, method, url string, body io.Reader) (*Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	request := Request{
		Request: req,
		Meta:    make(map[string]interface{}),
	}

	return &request, nil
}
