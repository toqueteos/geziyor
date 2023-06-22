package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponse_JoinURL(t *testing.T) {
	ctx := context.Background()
	req, _ := NewRequest(ctx, "GET", "https://localhost.com/test/a.html", nil)
	resp := Response{Request: req}

	joinedURL := resp.JoinURL("/source")
	assert.Equal(t, "https://localhost.com/source", joinedURL)
}
