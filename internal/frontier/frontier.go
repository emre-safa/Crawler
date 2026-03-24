package frontier

import (
	"container/heap"
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emre-safa/crawler/internal/types"
)

// Frontier is a thread-safe priority queue with an embedded visited set
// and back-pressure support. It is the core scheduling data structure.
type Frontier struct {
	mu       sync.Mutex
	cond     *sync.Cond
	h        crawlHeap
	visited  map[string]time.Time // canonical URL -> fetch time (zero means queued but not fetched)
	blocked  map[string]bool      // blocked domains
	maxSize  int
	closed   bool
}

// New creates a Frontier with the given maximum queue size for back-pressure.
func New(maxSize int) *Frontier {
	f := &Frontier{
		visited: make(map[string]time.Time),
		blocked: make(map[string]bool),
		maxSize: maxSize,
	}
	f.cond = sync.NewCond(&f.mu)
	heap.Init(&f.h)
	return f
}

// Push adds a CrawlItem if its URL has not been visited and its domain is not blocked.
// Blocks if the frontier is at capacity (back-pressure). Returns false if the item was rejected.
func (f *Frontier) Push(item types.CrawlItem) bool {
	canonical := Canonicalize(item.URL)
	if canonical == "" {
		return false
	}
	item.URL = canonical

	domain := DomainOf(canonical)

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return false
	}
	if f.blocked[domain] {
		return false
	}
	if _, seen := f.visited[canonical]; seen {
		return false
	}

	// Back-pressure: wait until there's room
	for f.h.Len() >= f.maxSize && !f.closed {
		f.cond.Wait()
	}
	if f.closed {
		return false
	}

	f.visited[canonical] = time.Time{} // mark as queued (not yet fetched)
	heap.Push(&f.h, item)
	f.cond.Broadcast() // wake any waiting Pop callers
	return true
}

// Pop removes and returns the highest-priority item. Blocks if the queue is empty.
// Returns (item, false) if the frontier is closed or the context is cancelled.
func (f *Frontier) Pop(ctx context.Context) (types.CrawlItem, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Wait until an item is available or we're done
	for f.h.Len() == 0 && !f.closed {
		// Watch context in a separate goroutine to unblock cond.Wait
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				f.mu.Lock()
				f.cond.Broadcast()
				f.mu.Unlock()
			case <-done:
			}
		}()

		f.cond.Wait()
		close(done)

		if ctx.Err() != nil {
			return types.CrawlItem{}, false
		}
	}

	if f.h.Len() == 0 {
		return types.CrawlItem{}, false
	}

	item := heap.Pop(&f.h).(types.CrawlItem)
	f.cond.Broadcast() // wake any Push callers blocked on back-pressure
	return item, true
}

// MarkFetched records that a URL has been successfully fetched.
func (f *Frontier) MarkFetched(u string) {
	canonical := Canonicalize(u)
	f.mu.Lock()
	f.visited[canonical] = time.Now()
	f.mu.Unlock()
}

// IsVisited returns true if the URL has been seen (queued or fetched).
func (f *Frontier) IsVisited(u string) bool {
	canonical := Canonicalize(u)
	f.mu.Lock()
	defer f.mu.Unlock()
	_, seen := f.visited[canonical]
	return seen
}

// BlockDomain marks a domain as blocked and removes all its URLs from the queue.
func (f *Frontier) BlockDomain(domain string) {
	domain = strings.ToLower(domain)
	f.mu.Lock()
	defer f.mu.Unlock()

	f.blocked[domain] = true

	// Rebuild heap without blocked domain
	var kept crawlHeap
	for _, item := range f.h {
		if DomainOf(item.URL) != domain {
			kept = append(kept, item)
		}
	}
	f.h = kept
	heap.Init(&f.h)
	f.cond.Broadcast()
}

// UnblockDomain removes a domain from the block list.
func (f *Frontier) UnblockDomain(domain string) {
	domain = strings.ToLower(domain)
	f.mu.Lock()
	f.blocked[domain] = false
	delete(f.blocked, domain)
	f.mu.Unlock()
}

// BlockedDomains returns the set of currently blocked domains.
func (f *Frontier) BlockedDomains() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	domains := make([]string, 0, len(f.blocked))
	for d := range f.blocked {
		domains = append(domains, d)
	}
	return domains
}

// Len returns the number of items currently in the queue.
func (f *Frontier) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.h.Len()
}

// Close shuts down the frontier, unblocking all waiting goroutines.
func (f *Frontier) Close() {
	f.mu.Lock()
	f.closed = true
	f.cond.Broadcast()
	f.mu.Unlock()
}

// Snapshot returns copies of the queue and visited set for checkpointing.
func (f *Frontier) Snapshot() ([]types.CrawlItem, map[string]time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	items := make([]types.CrawlItem, len(f.h))
	copy(items, f.h)

	visited := make(map[string]time.Time, len(f.visited))
	for k, v := range f.visited {
		visited[k] = v
	}
	return items, visited
}

// Restore loads a previously checkpointed state.
func (f *Frontier) Restore(items []types.CrawlItem, visited map[string]time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.visited = visited
	f.h = make(crawlHeap, len(items))
	copy(f.h, items)
	heap.Init(&f.h)
	f.cond.Broadcast()
}

// --- URL utilities ---

// Canonicalize normalizes a URL: lowercase scheme+host, strip fragment, sort query params.
func Canonicalize(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	// Remove trailing slash from path (except root)
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	// Sort query parameters for consistency
	q := u.Query()
	u.RawQuery = q.Encode()
	return u.String()
}

// DomainOf extracts the host (domain) from a canonical URL.
func DomainOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// --- Min-heap implementation ---

type crawlHeap []types.CrawlItem

func (h crawlHeap) Len() int            { return len(h) }
func (h crawlHeap) Less(i, j int) bool  { return h[i].Priority < h[j].Priority }
func (h crawlHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }

func (h *crawlHeap) Push(x interface{}) {
	*h = append(*h, x.(types.CrawlItem))
}

func (h *crawlHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
