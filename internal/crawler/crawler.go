package crawler

import (
	"context"
	"fmt"
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
		chromedp.Sleep(1500*time.Millisecond),
		c.stabilizePage(),
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

func (c *Client) stabilizePage() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		actions := []chromedp.Action{
			// Try lightweight popup dismissal (cookie/modal close buttons).
			chromedp.Evaluate(`(() => {
				const shouldClick = (el) => {
					const txt = (el.textContent || '').trim().toLowerCase();
					const aria = (el.getAttribute('aria-label') || '').trim().toLowerCase();
					const cls = (el.className || '').toString().toLowerCase();
					return txt === 'x' || txt === 'close' || txt === 'tutup' || txt === 'ok' || txt === 'accept' ||
						aria.includes('close') || aria.includes('tutup') || cls.includes('close');
				};
				document.querySelectorAll('button,[role="button"],.btn-close,.close').forEach((el) => {
					try { if (shouldClick(el)) el.click(); } catch (_) {}
				});
				return true;
			})()`, nil),
			// Promote common lazy-loading attributes so assets are fetched before snapshot.
			chromedp.Evaluate(`(() => {
				const attrs = ['data-src','data-lazy-src','data-original','data-url'];
				const srcsetAttrs = ['data-srcset','data-lazy-srcset'];
				document.querySelectorAll('img,source,iframe').forEach((el) => {
					for (const a of attrs) {
						const v = el.getAttribute(a);
						const cur = el.getAttribute('src');
						if (v && (!cur || cur === '' || cur.startsWith('data:'))) {
							el.setAttribute('src', v);
							break;
						}
					}
					for (const a of srcsetAttrs) {
						const v = el.getAttribute(a);
						if (v && !el.getAttribute('srcset')) {
							el.setAttribute('srcset', v);
							break;
						}
					}
					if (el.getAttribute('loading') === 'lazy') {
						el.setAttribute('loading', 'eager');
					}
				});
				return true;
			})()`, nil),
		}

		if err := chromedp.Run(ctx, actions...); err != nil {
			return fmt.Errorf("pre-stabilize actions failed: %w", err)
		}

		// Scroll down-up to trigger intersection/lazy observers.
		for _, y := range []int{0, 1200, 2600, 5000, 0} {
			js := fmt.Sprintf(`window.scrollTo(0, %d); true;`, y)
			if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil), chromedp.Sleep(500*time.Millisecond)); err != nil {
				return fmt.Errorf("scroll stabilize failed: %w", err)
			}
		}

		// Give images/scripts a bit more time to settle.
		if err := chromedp.Run(ctx, chromedp.Sleep(1200*time.Millisecond)); err != nil {
			return err
		}
		return nil
	})
}
