package crawler

import (
	"context"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type Config struct {
	Concurrency int
	Timeout     time.Duration
	UserAgent   string
	MinDelay    time.Duration
	MaxDelay    time.Duration
}

type Client struct {
	cfg Config
}

type Result struct {
	URL          string        `json:"url"`
	Host         string        `json:"host"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	Error        string        `json:"error,omitempty"`
	Duration     time.Duration `json:"duration"`
	CrawledAtUTC time.Time     `json:"crawled_at_utc"`
	HTML         string        `json:"-"`
}

func NewClient(cfg Config) *Client {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MinDelay < 0 {
		cfg.MinDelay = 0
	}
	if cfg.MaxDelay < cfg.MinDelay {
		cfg.MaxDelay = cfg.MinDelay
	}
	return &Client{cfg: cfg}
}

func (c *Client) Crawl(parent context.Context, urls []string) []Result {
	jobs := make(chan string)
	resultsCh := make(chan Result, len(urls))
	var wg sync.WaitGroup

	for i := 0; i < c.cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for u := range jobs {
				resultsCh <- c.crawlOne(parent, u, workerID)
			}
		}(i + 1)
	}

	go func() {
		defer close(jobs)
		for _, u := range urls {
			jobs <- u
		}
	}()

	wg.Wait()
	close(resultsCh)

	out := make([]Result, 0, len(urls))
	for r := range resultsCh {
		out = append(out, r)
	}
	return out
}

func (c *Client) crawlOne(parent context.Context, rawURL string, workerID int) Result {
	start := time.Now().UTC()
	u, err := url.Parse(rawURL)
	if err != nil {
		return Result{
			URL:          rawURL,
			Status:       "failed",
			Error:        err.Error(),
			CrawledAtUTC: start,
		}
	}

	delay := c.jitter(workerID)
	time.Sleep(delay)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(c.cfg.UserAgent),
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, c.cfg.Timeout)
	defer cancelTimeout()

	var html, title string
	err = chromedp.Run(timeoutCtx,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.Title(&title),
	)

	res := Result{
		URL:          rawURL,
		Host:         u.Hostname(),
		Title:        title,
		Duration:     time.Since(start),
		CrawledAtUTC: start,
		HTML:         html,
	}
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		return res
	}
	res.Status = "success"
	return res
}

func (c *Client) jitter(workerID int) time.Duration {
	if c.cfg.MaxDelay <= c.cfg.MinDelay {
		return c.cfg.MinDelay
	}
	seed := time.Now().UnixNano() + int64(workerID*1000)
	r := rand.New(rand.NewSource(seed))
	span := c.cfg.MaxDelay - c.cfg.MinDelay
	return c.cfg.MinDelay + time.Duration(r.Int63n(int64(span)+1))
}
