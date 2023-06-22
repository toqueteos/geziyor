package middleware

import (
	"context"
	"strconv"
	"sync"

	"github.com/temoto/robotstxt"
	"github.com/toqueteos/geziyor/client"
	"github.com/toqueteos/geziyor/internal"
	"github.com/toqueteos/geziyor/metrics"
)

// RobotsTxt middleware filters out requests forbidden by the robots.txt exclusion standard.
type RobotsTxt struct {
	ctx            context.Context
	metrics        *metrics.Metrics
	robotsDisabled bool
	client         *client.Client
	mut            sync.RWMutex
	robotsMap      map[string]*robotstxt.RobotsData
}

func NewRobotsTxt(ctx context.Context, client *client.Client, metrics *metrics.Metrics, robotsDisabled bool) RequestProcessor {
	return &RobotsTxt{
		ctx:            ctx,
		metrics:        metrics,
		robotsDisabled: robotsDisabled,
		client:         client,
		robotsMap:      make(map[string]*robotstxt.RobotsData),
	}
}

func (m *RobotsTxt) ProcessRequest(r *client.Request) {
	if m.robotsDisabled {
		return
	}

	// TODO: Locking like this improves performance but sometimes it causes duplicate requests to robots.txt
	m.mut.RLock()
	robotsData, exists := m.robotsMap[r.Host]
	m.mut.RUnlock()

	if !exists {
		robotsReq, err := client.NewRequest(m.ctx, "GET", r.URL.Scheme+"://"+r.Host+"/robots.txt", nil)
		if err != nil {
			return // Don't Do anything
		}

		m.metrics.RobotsTxtRequestCounter.Add(1)
		robotsResp, err := m.client.DoRequest(robotsReq)
		if err != nil {
			return // Don't Do anything
		}
		m.metrics.RobotsTxtResponseCounter.With("status", strconv.Itoa(robotsResp.StatusCode)).Add(1)

		robotsData, err = robotstxt.FromStatusAndBytes(robotsResp.StatusCode, robotsResp.Body)
		if err != nil {
			return // Don't Do anything
		}

		m.mut.Lock()
		m.robotsMap[r.Host] = robotsData
		m.mut.Unlock()
	}

	if !robotsData.TestAgent(r.URL.Path, r.UserAgent()) {
		m.metrics.RobotsTxtForbiddenCounter.With("method", r.Method).Add(1)
		internal.Logger.Println("Forbidden by robots.txt:", r.URL.String())
		r.Cancel()
	}
}
