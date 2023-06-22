package geziyor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/elazarl/goproxy"
	"github.com/fortytw2/leaktest"
	"github.com/stretchr/testify/assert"
	"github.com/toqueteos/geziyor"
	"github.com/toqueteos/geziyor/cache"
	"github.com/toqueteos/geziyor/cache/diskcache"
	"github.com/toqueteos/geziyor/client"
	"github.com/toqueteos/geziyor/export"
	"github.com/toqueteos/geziyor/internal"
	"github.com/toqueteos/geziyor/metrics"
)

func TestSimple(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/ip"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
		},
	}).Start(ctx)
}

func TestUserAgent(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/anything"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			var data map[string]interface{}
			err := json.Unmarshal(r.Body, &data)

			assert.NoError(t, err)
			assert.Equal(t, client.DefaultUserAgent, data["headers"].(map[string]interface{})["User-Agent"])
		},
	}).Start(ctx)
}

func TestCache(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/ip"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
			g.Exports <- string(r.Body)
			g.Get(ctx, "https://httpbingo.org/ip", nil)
		},
		Cache:       diskcache.New(".cache"),
		CachePolicy: cache.RFC2616,
	}).Start(ctx)
}

func TestQuotes(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"http://quotes.toscrape.com/"},
		ParseFunc: quotesParse,
		Exporters: []export.Exporter{&export.JSONLine{FileName: "1.jsonl"}, &export.JSON{FileName: "2.json"}},
	}).Start(ctx)
}

func quotesParse(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
	r.HTMLDoc.Find("div.quote").Each(func(i int, s *goquery.Selection) {
		// Export Data
		g.Exports <- map[string]interface{}{
			"number": i,
			"text":   s.Find("span.text").Text(),
			"author": s.Find("small.author").Text(),
			"tags": s.Find("div.tags > a.tag").Map(func(_ int, s *goquery.Selection) string {
				return s.Text()
			}),
		}
	})

	// Next Page
	if href, ok := r.HTMLDoc.Find("li.next > a").Attr("href"); ok {
		absoluteURL, _ := r.Request.URL.Parse(href)
		g.Get(ctx, absoluteURL.String(), quotesParse)
	}
}

func TestAllLinks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	ctx := context.Background()

	geziyor.NewGeziyor(ctx, &geziyor.Options{
		AllowedDomains: []string{"books.toscrape.com"},
		StartURLs:      []string{"http://books.toscrape.com/"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			g.Exports <- []string{r.Request.URL.String()}
			r.HTMLDoc.Find("a").Each(func(i int, s *goquery.Selection) {
				if href, ok := s.Attr("href"); ok {
					absoluteURL, _ := r.Request.URL.Parse(href)
					g.Get(ctx, absoluteURL.String(), g.Opt.ParseFunc)
				}
			})
		},
		Exporters:   []export.Exporter{&export.CSV{}},
		MetricsType: metrics.Prometheus,
	}).Start(ctx)
}

func TestStartRequestsFunc(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			g.Get(ctx, "http://quotes.toscrape.com/", nil)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			r.HTMLDoc.Find("a").Each(func(_ int, s *goquery.Selection) {
				g.Exports <- s.AttrOr("href", "")
			})
		},
		Exporters: []export.Exporter{&export.JSON{}},
	}).Start(ctx)
}

func TestGetRendered(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			g.GetRendered(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
			fmt.Println(r.Request.URL.String(), r.Header)
		},
		//URLRevisitEnabled: true,
	}).Start(ctx)
}

func TestGetRenderedCustomActions(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			req, _ := client.NewRequest(ctx, "GET", "https://httpbingo.org/anything", nil)
			req.Rendered = true
			req.Actions = []chromedp.Action{
				chromedp.Navigate("https://httpbingo.org/anything"),
				chromedp.WaitReady(":root"),
				chromedp.ActionFunc(func(ctx context.Context) error {
					node, err := dom.GetDocument().Do(ctx)
					if err != nil {
						return err
					}
					body, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
					fmt.Println("HOLAAA", body)
					return err
				}),
			}
			g.Do(req, g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			assert.Equal(t, 200, r.StatusCode)
			fmt.Println(string(r.Body))
			fmt.Println(r.Request.URL.String(), r.Header)
		},
		// This will make only visit and nothing more.
		//PreActions: []chromedp.Action{
		//	chromedp.Navigate("https://httpbingo.org/anything"),
		//},
	}).Start(ctx)
}

func TestGetRenderedCookie(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Cookie")))
	}))
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			req, err := client.NewRequest(ctx, "GET", testServer.URL, nil)
			if err != nil {
				internal.Logger.Printf("Request creating error %v\n", err)
				return
			}
			req.Header.Set("Cookie", "key=value")
			req.Rendered = true
			g.Do(req, g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			assert.Contains(t, string(r.Body), "key=value")
		},
	}).Start(ctx)
}

// Run chrome headless instance to test this
// func TestGetRenderedRemoteAllocator(t *testing.T) {
// 	ctx := context.Background()
// 	geziyor.NewGeziyor(ctx, &geziyor.Options{
// 		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
// 			g.GetRendered(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
// 		},
// 		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
// 			fmt.Println(string(r.Body))
// 			fmt.Println(r.Request.URL.String(), r.Header)
// 		},
// 		BrowserEndpoint: "ws://localhost:3000",
// 	}).Start(ctx)
// }

func TestHEADRequest(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			g.Head(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
		},
	}).Start(ctx)
}

type PostBody struct {
	UserName string `json:"user_name"`
	Message  string `json:"message"`
}

func TestPostJson(_ *testing.T) {
	postBody := &PostBody{
		UserName: "Juan Valdez",
		Message:  "Best coffee in town",
	}
	payloadBuf := new(bytes.Buffer)
	json.NewEncoder(payloadBuf).Encode(postBody)

	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			g.Post(ctx, "https://reqbin.com/echo/post/json", payloadBuf, g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
			g.Exports <- string(r.Body)
		},
		Exporters: []export.Exporter{&export.JSON{FileName: "post_json.json"}},
	}).Start(ctx)
}

func TestPostFormUrlEncoded(_ *testing.T) {
	var postForm url.Values
	postForm.Set("user_name", "Juan Valdez")
	postForm.Set("message", "Enjoy a good coffee!")

	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			g.Post(ctx, "https://reqbin.com/echo/post/form", strings.NewReader(postForm.Encode()), g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			fmt.Println(string(r.Body))
			g.Exports <- map[string]interface{}{
				"host":            r.Request.Host,
				"h1":              r.HTMLDoc.Find("h1").Text(),
				"entire_response": string(r.Body),
			}
		},
		Exporters: []export.Exporter{&export.JSON{FileName: "post_form.json"}},
	}).Start(ctx)
}

func TestCookies(t *testing.T) {
	ctx := context.Background()

	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"http://quotes.toscrape.com/login"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			if len(g.Client.Cookies(r.Request.URL.String())) == 0 {
				t.Fatal("Cookies is Empty")
			}
		},
	}).Start(ctx)

	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"http://quotes.toscrape.com/login"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			if len(g.Client.Cookies(r.Request.URL.String())) != 0 {
				t.Fatal("Cookies exist")
			}
		},
		CookiesDisabled: true,
	}).Start(ctx)
}

func TestBasicAuth(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			req, _ := client.NewRequest(ctx, "GET", "https://httpbingo.org/anything", nil)
			req.SetBasicAuth("username", "password")
			g.Do(req, nil)
		},
		MetricsType: metrics.ExpVar,
	}).Start(ctx)
}

func TestRedirect(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/absolute-redirect/1"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			//t.Fail()
		},
		MaxRedirect: -1,
	}).Start(ctx)

	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/absolute-redirect/1"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			if r.StatusCode == 302 {
				t.Fail()
			}
		},
		MaxRedirect: 0,
	}).Start(ctx)
}

func TestConcurrentRequests(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs:                   []string{"https://httpbingo.org/delay/1", "https://httpbingo.org/delay/2"},
		ConcurrentRequests:          1,
		ConcurrentRequestsPerDomain: 1,
	}).Start(ctx)
}

func TestRobots(t *testing.T) {
	ctx := context.Background()
	defer leaktest.Check(t)()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs: []string{"https://httpbingo.org/deny"},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			t.Error("/deny should be blocked by robots.txt middleware")
		},
	}).Start(ctx)
}

func TestPassMetadata(t *testing.T) {
	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			req, _ := client.NewRequest(ctx, "GET", "https://httpbingo.org/anything", nil)
			req.Meta["key"] = "value"
			g.Do(req, g.Opt.ParseFunc)
		},
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			assert.Equal(t, r.Request.Meta["key"], "value")
		},
	}).Start(ctx)
}

func TestProxy(t *testing.T) {
	// Setup fake proxy server
	testHeaderKey := "Geziyor"
	testHeaderVal := "value"
	proxy := goproxy.NewProxyHttpServer()
	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		r.Header.Set(testHeaderKey, testHeaderVal)
		return r, nil
	})
	ts := httptest.NewServer(proxy)
	defer ts.Close()

	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartURLs:         []string{"http://httpbingo.org/anything"},
		ProxyFunc:         client.RoundRobinProxy(ts.URL),
		RobotsTxtDisabled: true,
		ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
			var data map[string]interface{}
			err := json.Unmarshal(r.Body, &data)
			assert.NoError(t, err)
			// Check header set
			assert.Equal(t, testHeaderVal, data["headers"].(map[string]interface{})[testHeaderKey])
		},
	}).Start(ctx)
}

// Make sure to increase open file descriptor limits before running
func BenchmarkRequests(b *testing.B) {

	// Create Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, client")
	}))
	ts.Client().Transport = client.NewClient(&client.Options{
		MaxBodySize:    client.DefaultMaxBody,
		RetryTimes:     client.DefaultRetryTimes,
		RetryHTTPCodes: client.DefaultRetryHTTPCodes,
	}).Transport
	defer ts.Close()

	// As we don't benchmark creating a server, reset timer.
	b.ResetTimer()

	ctx := context.Background()
	geziyor.NewGeziyor(ctx, &geziyor.Options{
		StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
			// Create Synchronized request to benchmark requests accurately.
			req, _ := client.NewRequest(ctx, "GET", ts.URL, nil)
			req.Synchronized = true

			// We only bench here !
			for i := 0; i < b.N; i++ {
				g.Do(req, nil)
			}
		},
		URLRevisitEnabled: true,
		LogDisabled:       true,
	}).Start(ctx)
}

func BenchmarkWhole(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		geziyor.NewGeziyor(ctx, &geziyor.Options{
			AllowedDomains: []string{"quotes.toscrape.com"},
			StartURLs:      []string{"http://quotes.toscrape.com/"},
			ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
				g.Exports <- []string{r.Request.URL.String()}
				r.HTMLDoc.Find("a").Each(func(i int, s *goquery.Selection) {
					if href, ok := s.Attr("href"); ok {
						absoluteURL, _ := r.Request.URL.Parse(href)
						g.Get(ctx, absoluteURL.String(), g.Opt.ParseFunc)
					}
				})
			},
			Exporters: []export.Exporter{&export.CSV{}},
			//MetricsType: metrics.Prometheus,
			LogDisabled: true,
		}).Start(ctx)
	}
}
