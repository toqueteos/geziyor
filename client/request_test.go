package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeta(t *testing.T) {
	req, err := NewRequest(context.Background(), "GET", "https://github.com/toqueteos/geziyor", nil)
	assert.NoError(t, err)
	req.Meta["key"] = "value"

	assert.Equal(t, req.Meta["key"], "value")
}
