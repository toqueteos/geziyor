package middleware

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRandomDelay(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	delay := time.Millisecond * 1000
	min := float64(delay) * 0.5
	max := float64(delay) * 1.5
	randomDelay := rand.Intn(int(max-min)) + int(min)

	assert.True(t, time.Duration(randomDelay).Seconds() < 1.5)
	assert.True(t, time.Duration(randomDelay).Seconds() > 0.5)
}
