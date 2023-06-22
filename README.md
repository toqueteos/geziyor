[![Go Reference](https://pkg.go.dev/badge/github.com/toqueteos/geziyor.svg)](https://pkg.go.dev/github.com/toqueteos/geziyor)

# Geziyor (forked)

Geziyor is a web crawling and web scraping framework.

## About this fork

- Updated default chromedp actions to wait for network requests to finish
- Added `context.Context` support for easy cancellation
- Lower timeout values

## Features

- **JS Rendering**
- 5.000+ requests/second
- Caching (Memory/Disk/LevelDB)
- Automatic Data Exporting (JSON, JSONL, CSV, or custom)
- Metrics (Prometheus, Expvar, or custom)
- Limit Concurrency (Global/Per Domain)
- Request Delays (Constant/Randomized)
- Cookies, Middlewares, robots.txt
- Automatic response decoding to UTF-8
- Proxy management (Single, Round-Robin, Custom)

See scraper [Options](https://pkg.go.dev/github.com/toqueteos/geziyor#Options) for all custom settings.

## Usage

This example extracts all quotes from _[quotes.toscrape.com](http://quotes.toscrape.com)_ and exports to JSON file.

```go
func main() {
    ctx := context.TODO()
    geziyor.NewGeziyor(ctx, &geziyor.Options{
        StartURLs: []string{"http://quotes.toscrape.com/"},
        ParseFunc: quotesParse,
        Exporters: []export.Exporter{&export.JSON{}},
    }).Start(ctx)
}

func quotesParse(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
    r.HTMLDoc.Find("div.quote").Each(func(i int, s *goquery.Selection) {
        g.Exports <- map[string]interface{}{
            "text":   s.Find("span.text").Text(),
            "author": s.Find("small.author").Text(),
        }
    })
    if href, ok := r.HTMLDoc.Find("li.next > a").Attr("href"); ok {
        g.Get(ctx, r.JoinURL(href), quotesParse)
    }
}
```

See [tests](https://github.com/toqueteos/geziyor/blob/master/geziyor_test.go) for more usage examples.

## Installation

```bash
go get -u github.com/toqueteos/geziyor
```

If you want to make JS rendered requests, a local Chrome is required.

Alternatively you can use any Chromium-based headless docker image such as the one available from the [Ferret project](https://www.montferret.dev):

```bash
docker run --rm -d -p 9222:9222 montferret/chromium
```

**Don't forget to set `Options.BrowserEndpoint`!**

**NOTE**: macOS limits the maximum number of open file descriptors.
If you want to make concurrent requests over 256, you need to increase limits.
Read [this](https://wilsonmar.github.io/maximum-limits/) for more.

## Making Normal Requests

Initial requests start with `StartURLs []string` field in `Options`.
Geziyor makes concurrent requests to those URLs.
After reading response, `ParseFunc func(g *Geziyor, r *Response)` called.

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartURLs: []string{"https://httpbingo.org/ip"},
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        fmt.Println(string(r.Body))
    },
}).Start(ctx)
```

If you want to manually create first requests, set `StartRequestsFunc`.
`StartURLs` won't be used if you create requests manually.
You can make requests using `Geziyor` [methods](https://godoc.org/github.com/toqueteos/geziyor#Geziyor):

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
    	g.Get(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
        g.Head(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
    },
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        fmt.Println(string(r.Body))
    },
}).Start(ctx)
```

## Making JS Rendered Requests

JS Rendered requests can be made using `GetRendered` method.

By default, geziyor tries to launch a local Chrome instance, if there's one available locally.

You can set the `BrowserEndpoint` option to use connect to a different Chrome instance.

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
        g.GetRendered(ctx, "https://httpbingo.org/anything", g.Opt.ParseFunc)
    },
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        fmt.Println(string(r.Body))
    },
    BrowserEndpoint: "ws://localhost:9292",
}).Start(ctx)
```

## Extracting Data

We can extract HTML elements using `response.HTMLDoc`. HTMLDoc is Goquery's [Document](https://godoc.org/github.com/PuerkitoBio/goquery#Document).

HTMLDoc can be accessible on Response if response is HTML and can be parsed using Go's built-in HTML [parser](https://godoc.org/golang.org/x/net/html#Parse)
If response isn't HTML, `response.HTMLDoc` would be `nil`.

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartURLs: []string{"http://quotes.toscrape.com/"},
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        r.HTMLDoc.Find("div.quote").Each(func(_ int, s *goquery.Selection) {
            log.Println(s.Find("span.text").Text(), s.Find("small.author").Text())
        })
    },
}).Start(ctx)
```

## Exporting Data

You can export data automatically using exporters. Just send data to `Geziyor.Exports` chan.
[Available exporters](https://godoc.org/github.com/toqueteos/geziyor/export)

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartURLs: []string{"http://quotes.toscrape.com/"},
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        r.HTMLDoc.Find("div.quote").Each(func(_ int, s *goquery.Selection) {
            g.Exports <- map[string]interface{}{
                "text":   s.Find("span.text").Text(),
                "author": s.Find("small.author").Text(),
            }
        })
    },
    Exporters: []export.Exporter{&export.JSON{}},
}).Start(ctx)
```

## Custom Requests - Passing Metadata To Callbacks

You can create custom requests with `client.NewRequest`

Use that request on `geziyor.Do(request, callback)`

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartRequestsFunc: func(ctx context.Context, g *geziyor.Geziyor) {
        req, _ := client.NewRequest(ctx, "GET", "https://httpbingo.org/anything", nil)
        req.Meta["key"] = "value"
        g.Do(req, g.Opt.ParseFunc)
    },
    ParseFunc: func(ctx context.Context, g *geziyor.Geziyor, r *client.Response) {
        fmt.Println("This is our data from request: ", r.Request.Meta["key"])
    },
}).Start(ctx)
```

## Proxy - Use proxy per request

If you want to use proxy for your requests, and you have 1 proxy, you can just set these env values:
`HTTP_PROXY`
`HTTPS_PROXY`
And geziyor will use those proxies.

Also, you can use in-order proxy per request by setting `ProxyFunc` option to `client.RoundRobinProxy`
Or any custom proxy selection function that you want. See `client/proxy.go` on how to implement that kind of custom proxy selection function.

Proxies can be HTTP, HTTPS and SOCKS5.

Note: If you use `http` scheme for proxy, It'll be used for http requests and not for https requests.

```go
geziyor.NewGeziyor(ctx, &geziyor.Options{
    StartURLs:         []string{"http://httpbingo.org/anything"},
    ParseFunc:         parseFunc,
    ProxyFunc:         client.RoundRobinProxy("http://some-http-proxy.com", "https://some-https-proxy.com", "socks5://some-socks5-proxy.com"),
}).Start(ctx)
```

## Benchmark

See [tests](https://github.com/toqueteos/geziyor/blob/master/geziyor_test.go) for this benchmark function:

```bash
>> go test -run none -bench Requests -benchtime 10s
goos: linux
goarch: amd64
pkg: github.com/toqueteos/geziyor
cpu: AMD Ryzen 7 7700X 8-Core Processor
BenchmarkRequests-16              362724             38632 ns/op
PASS
ok      github.com/toqueteos/geziyor    14.352s
```
