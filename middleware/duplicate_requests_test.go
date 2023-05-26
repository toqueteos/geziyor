package middleware

import (
	"github.com/stretchr/testify/assert"
	"github.com/toqueteos/geziyor/client"
	"strings"
	"testing"
)

func TestDuplicateRequests_ProcessRequest(t *testing.T) {
	longURL := "https://example.com" + strings.Repeat("/path", 50)
	req, err := client.NewRequest("GET", longURL, nil)
	assert.NoError(t, err)
	req2, err := client.NewRequest("GET", longURL, nil)
	assert.NoError(t, err)

	duplicateRequestsProcessor := DuplicateRequests{RevisitEnabled: false}
	duplicateRequestsProcessor.ProcessRequest(req)
	duplicateRequestsProcessor.ProcessRequest(req2)
	duplicateRequestsProcessor.ProcessRequest(req2)
	duplicateRequestsProcessor.ProcessRequest(req2)
}
