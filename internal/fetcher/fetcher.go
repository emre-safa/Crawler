package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/emre-safa/crawler/internal/types"
)

// Config holds the settings needed by the Fetcher.
type Config struct {
	FetchTimeout time.Duration
	MaxRedirects int
	MaxBodySize  int64
	DefaultRate  int // default requests per second per domain
}

// Fetcher handles HTTP requests with per-domain rate limiting.
type Fetcher struct {
	client   *http.Client
	cfg      Config
	mu       sync.Mutex
	limiters map[string]*domainLimiter
}

// domainLimiter is a simple token-bucket rate limiter per domain.
type domainLimiter struct {
	mu      sync.Mutex
	lastReq time.Time
	blocked bool
	rate    int // requests per second (0 means use default)
}

// New creates a Fetcher with the given configuration.
func New(cfg Config) *Fetcher {
	client := &http.Client{
		Timeout: cfg.FetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return fmt.Errorf("too many redirects (%d)", cfg.MaxRedirects)
			}
			return nil
		},
	}
	if cfg.DefaultRate <= 0 {
		cfg.DefaultRate = 2
	}
	return &Fetcher{
		client:   client,
		cfg:      cfg,
		limiters: make(map[string]*domainLimiter),
	}
}

// Fetch downloads the page at item.URL, respecting rate limits.
// rateLimit overrides the default per-domain rate if > 0.
func (f *Fetcher) Fetch(ctx context.Context, item types.CrawlItem, rateLimit int) (body string, statusCode int, err error) {
	domain := domainOf(item.URL)

	if err := f.waitRateLimit(ctx, domain, rateLimit); err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("fetcher: create request: %w", err)
	}
	req.Header.Set("User-Agent", "CrawlerBot/1.0 (educational project)")
	req.Header.Set("Accept", "text/html")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("fetcher: GET %s: %w", item.URL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, f.cfg.MaxBodySize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("fetcher: read body %s: %w", item.URL, err)
	}

	return string(data), resp.StatusCode, nil
}

func (f *Fetcher) waitRateLimit(ctx context.Context, domain string, rateLimit int) error {
	f.mu.Lock()
	lim, ok := f.limiters[domain]
	if !ok {
		lim = &domainLimiter{}
		f.limiters[domain] = lim
	}
	f.mu.Unlock()

	lim.mu.Lock()
	defer lim.mu.Unlock()

	if lim.blocked {
		return fmt.Errorf("fetcher: domain %s is blocked", domain)
	}

	rate := rateLimit
	if rate <= 0 {
		rate = f.cfg.DefaultRate
	}
	minInterval := time.Second / time.Duration(rate)

	elapsed := time.Since(lim.lastReq)
	if elapsed < minInterval {
		wait := minInterval - elapsed
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	lim.lastReq = time.Now()
	return nil
}

// BlockDomain marks a domain as blocked in the rate limiter.
func (f *Fetcher) BlockDomain(domain string) {
	f.mu.Lock()
	lim, ok := f.limiters[domain]
	if !ok {
		lim = &domainLimiter{}
		f.limiters[domain] = lim
	}
	f.mu.Unlock()

	lim.mu.Lock()
	lim.blocked = true
	lim.mu.Unlock()
}

// UnblockDomain removes the block on a domain.
func (f *Fetcher) UnblockDomain(domain string) {
	f.mu.Lock()
	lim, ok := f.limiters[domain]
	f.mu.Unlock()
	if !ok {
		return
	}
	lim.mu.Lock()
	lim.blocked = false
	lim.mu.Unlock()
}

func domainOf(rawURL string) string {
	start := 0
	if idx := indexOf(rawURL, "://"); idx >= 0 {
		start = idx + 3
	}
	end := len(rawURL)
	for i := start; i < len(rawURL); i++ {
		if rawURL[i] == '/' || rawURL[i] == '?' || rawURL[i] == '#' {
			end = i
			break
		}
	}
	return rawURL[start:end]
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
