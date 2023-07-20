package geziyor

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/toqueteos/geziyor/cache"
	"github.com/toqueteos/geziyor/client"
	"github.com/toqueteos/geziyor/export"
	"github.com/toqueteos/geziyor/internal"
	"github.com/toqueteos/geziyor/metrics"
	"github.com/toqueteos/geziyor/middleware"
	"golang.org/x/time/rate"

	"io"
	"io/ioutil"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
)

// Geziyor is our main scraper type
type Geziyor struct {
	Opt     *Options
	Client  *client.Client
	Exports chan interface{}

	metrics        *metrics.Metrics
	reqMiddlewares []middleware.RequestProcessor
	resMiddlewares []middleware.ResponseProcessor
	rateLimiter    *rate.Limiter
	wgRequests     sync.WaitGroup
	wgExporters    sync.WaitGroup
	semGlobal      chan struct{}
	semHosts       struct {
		sync.RWMutex
		hostSems map[string]chan struct{}
	}
	shutdown bool
}

// NewGeziyor creates new Geziyor with default values.
// If options provided, options
func NewGeziyor(ctx context.Context, opt *Options) *Geziyor {

	// Default Options
	if opt.UserAgent == "" {
		opt.UserAgent = client.DefaultUserAgent
	}
	if opt.MaxBodySize == 0 {
		opt.MaxBodySize = client.DefaultMaxBody
	}
	if opt.RetryTimes == 0 {
		opt.RetryTimes = client.DefaultRetryTimes
	}
	if len(opt.RetryHTTPCodes) == 0 {
		opt.RetryHTTPCodes = client.DefaultRetryHTTPCodes
	}

	geziyor := &Geziyor{
		Opt:     opt,
		Exports: make(chan interface{}, 1),
		reqMiddlewares: []middleware.RequestProcessor{
			&middleware.AllowedDomains{AllowedDomains: opt.AllowedDomains},
			&middleware.DuplicateRequests{RevisitEnabled: opt.URLRevisitEnabled},
			&middleware.Headers{UserAgent: opt.UserAgent},
			middleware.NewDelay(opt.RequestDelayRandomize, opt.RequestDelay),
		},
		resMiddlewares: []middleware.ResponseProcessor{
			&middleware.ParseHTML{ParseHTMLDisabled: opt.ParseHTMLDisabled},
			&middleware.LogStats{LogDisabled: opt.LogDisabled},
		},
		metrics: metrics.NewMetrics(opt.MetricsType),
	}

	// Client
	geziyor.Client = client.NewClient(&client.Options{
		MaxBodySize:           opt.MaxBodySize,
		CharsetDetectDisabled: opt.CharsetDetectDisabled,
		RetryTimes:            opt.RetryTimes,
		RetryHTTPCodes:        opt.RetryHTTPCodes,
		RemoteAllocatorURL:    opt.BrowserEndpoint,
		AllocatorOptions:      chromedp.DefaultExecAllocatorOptions[:],
		ProxyFunc:             opt.ProxyFunc,
		PreActions:            opt.PreActions,
	})
	if opt.Cache != nil {
		geziyor.Client.Transport = &cache.Transport{
			Policy:              opt.CachePolicy,
			Transport:           geziyor.Client.Transport,
			Cache:               opt.Cache,
			MarkCachedResponses: true,
		}
	}
	if opt.Timeout != 0 {
		geziyor.Client.Timeout = opt.Timeout
	}
	if !opt.CookiesDisabled {
		geziyor.Client.Jar, _ = cookiejar.New(nil)
	}
	if opt.MaxRedirect != 0 {
		geziyor.Client.CheckRedirect = client.NewRedirectionHandler(opt.MaxRedirect)
	}

	// Concurrency
	if opt.RequestsPerSecond != 0 {
		geziyor.rateLimiter = rate.NewLimiter(rate.Limit(opt.RequestsPerSecond), int(opt.RequestsPerSecond))
	}
	if opt.ConcurrentRequests != 0 {
		geziyor.semGlobal = make(chan struct{}, opt.ConcurrentRequests)
	}
	if opt.ConcurrentRequestsPerDomain != 0 {
		geziyor.semHosts = struct {
			sync.RWMutex
			hostSems map[string]chan struct{}
		}{hostSems: make(map[string]chan struct{})}
	}

	// Base Middlewares
	metricsMiddleware := &middleware.Metrics{Metrics: geziyor.metrics}
	geziyor.reqMiddlewares = append(geziyor.reqMiddlewares, metricsMiddleware)
	geziyor.resMiddlewares = append(geziyor.resMiddlewares, metricsMiddleware)

	robotsMiddleware := middleware.NewRobotsTxt(ctx, geziyor.Client, geziyor.metrics, opt.RobotsTxtDisabled)
	geziyor.reqMiddlewares = append(geziyor.reqMiddlewares, robotsMiddleware)

	// Custom Middlewares
	geziyor.reqMiddlewares = append(geziyor.reqMiddlewares, opt.RequestMiddlewares...)
	geziyor.resMiddlewares = append(geziyor.resMiddlewares, opt.ResponseMiddlewares...)

	// Logging
	if opt.LogDisabled {
		internal.Logger.SetOutput(ioutil.Discard)
	} else {
		internal.Logger.SetOutput(os.Stdout)
	}

	return geziyor
}

// Start starts scraping
func (g *Geziyor) Start(ctx context.Context) {
	internal.Logger.Println("Scraping Started")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Metrics
	if g.Opt.MetricsType == metrics.Prometheus || g.Opt.MetricsType == metrics.ExpVar {
		metricsServer := metrics.StartMetricsServer(g.Opt.MetricsType)
		defer metricsServer.Close()
	}

	// Start Exporters
	g.startExporters()

	// Wait for SIGINT (interrupt) signal.
	shutdownDoneChan := make(chan struct{})
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	go g.interruptSignalWaiter(ctx, shutdownDoneChan)

	// Start Requests
	if g.Opt.StartRequestsFunc != nil {
		g.Opt.StartRequestsFunc(ctx, g)
	} else {
		for _, startURL := range g.Opt.StartURLs {
			g.Get(ctx, startURL, g.Opt.ParseFunc)
		}
	}

	g.wgRequests.Wait()
	close(g.Exports)
	g.wgExporters.Wait()
	shutdownDoneChan <- struct{}{}
	internal.Logger.Println("Scraping Finished")
}

// Get issues a GET to the specified URL.
func (g *Geziyor) Get(ctx context.Context, url string, callback ParseFunc) {
	req, err := client.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		internal.Logger.Printf("Request creating error %v\n", err)
		return
	}
	g.Do(req, callback)
}

// GetRendered issues GET request using headless browser
// Opens up a new Chrome instance, makes request, waits for rendering HTML DOM and closed.
// Rendered requests only supported for GET requests.
func (g *Geziyor) GetRendered(ctx context.Context, url string, callback ParseFunc) {
	req, err := client.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		internal.Logger.Printf("Request creating error %v\n", err)
		return
	}
	req.Rendered = true
	g.Do(req, callback)
}

// Head issues a HEAD to the specified URL
func (g *Geziyor) Head(ctx context.Context, url string, callback ParseFunc) {
	req, err := client.NewRequest(ctx, "HEAD", url, nil)
	if err != nil {
		internal.Logger.Printf("Request creating error %v\n", err)
		return
	}
	g.Do(req, callback)
}

// Post issues a POST to the specified URL
func (g *Geziyor) Post(ctx context.Context, url string, body io.Reader, callback ParseFunc) {
	req, err := client.NewRequest(ctx, "POST", url, body)
	if err != nil {
		internal.Logger.Printf("Request creating error %v\n", err)
		return
	}
	g.Do(req, callback)
}

// Do sends an HTTP request
func (g *Geziyor) Do(req *client.Request, callback ParseFunc) {
	if g.shutdown {
		return
	}
	g.wgRequests.Add(1)
	if req.Synchronized {
		g.do(req, callback)
	} else {
		go g.do(req, callback)
	}
}

// Do sends an HTTP request
func (g *Geziyor) do(req *client.Request, callback ParseFunc) {
	g.acquireSem(req)
	defer g.releaseSem(req)
	defer g.wgRequests.Done()
	defer g.recoverMe()

	for _, middlewareFunc := range g.reqMiddlewares {
		middlewareFunc.ProcessRequest(req)
		if req.Cancelled {
			return
		}
	}

	res, err := g.Client.DoRequest(req)
	if err != nil {
		if g.Opt.ErrorFunc != nil {
			g.Opt.ErrorFunc(req.Context(), g, req, err)
		} else {
			internal.Logger.Println(err)
		}
		return
	}

	for _, middlewareFunc := range g.resMiddlewares {
		middlewareFunc.ProcessResponse(res)
	}

	// Callbacks
	if callback != nil {
		callback(req.Context(), g, res)
	} else {
		if g.Opt.ParseFunc != nil {
			g.Opt.ParseFunc(req.Context(), g, res)
		}
	}
}

func (g *Geziyor) acquireSem(req *client.Request) {
	if g.rateLimiter != nil {
		_ = g.rateLimiter.Wait(req.Context())
	}
	if g.Opt.ConcurrentRequests != 0 {
		g.semGlobal <- struct{}{}
	}
	if g.Opt.ConcurrentRequestsPerDomain != 0 {
		g.semHosts.RLock()
		hostSem, exists := g.semHosts.hostSems[req.Host]
		g.semHosts.RUnlock()
		if !exists {
			hostSem = make(chan struct{}, g.Opt.ConcurrentRequestsPerDomain)
			g.semHosts.Lock()
			g.semHosts.hostSems[req.Host] = hostSem
			g.semHosts.Unlock()
		}
		hostSem <- struct{}{}
	}
}

func (g *Geziyor) releaseSem(req *client.Request) {
	if g.Opt.ConcurrentRequests != 0 {
		<-g.semGlobal
	}
	if g.Opt.ConcurrentRequestsPerDomain != 0 {
		g.semHosts.RLock()
		hostSem := g.semHosts.hostSems[req.Host]
		g.semHosts.RUnlock()
		<-hostSem
	}
}

// recoverMe prevents scraping being crashed.
// Logs error and stack trace
func (g *Geziyor) recoverMe() {
	if r := recover(); r != nil {
		internal.Logger.Println(r, string(debug.Stack()))
		g.metrics.PanicCounter.Add(1)
	}
}

// interruptSignalWaiter waits data from provided channels and stops scraper if shutdownChan channel receives SIGINT
func (g *Geziyor) interruptSignalWaiter(ctx context.Context, shutdownDoneChan chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// tick
		case <-ctx.Done():
			internal.Logger.Println("Received SIGINT, shutting down gracefully. Send again to force")
			g.shutdown = true
		case <-shutdownDoneChan:
			return
		}
	}
}

func (g *Geziyor) startExporters() {
	if len(g.Opt.Exporters) != 0 {
		var exporterChans []chan interface{}

		g.wgExporters.Add(len(g.Opt.Exporters))
		for _, exporter := range g.Opt.Exporters {
			exporterChan := make(chan interface{})
			exporterChans = append(exporterChans, exporterChan)
			go func(exporter export.Exporter) {
				defer g.wgExporters.Done()
				if err := exporter.Export(exporterChan); err != nil {
					internal.Logger.Printf("exporter error: %s\n", err)
				}
			}(exporter)
		}
		go func() {
			// When exports closed, close the exporter chans.
			// Exports chan will be closed after all requests are handled.
			defer func() {
				for _, exporterChan := range exporterChans {
					close(exporterChan)
				}
			}()
			// Send incoming data from exports to all of the exporter's chans
			for data := range g.Exports {
				for _, exporterChan := range exporterChans {
					exporterChan <- data
				}
			}
		}()
	} else {
		g.wgExporters.Add(1)
		go func() {
			for range g.Exports {
			}
			g.wgExporters.Done()
		}()
	}
}
