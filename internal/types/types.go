package types

import "time"

// CrawlItem represents a URL in the frontier queue.
type CrawlItem struct {
	URL       string
	OriginURL string  // seed or parent URL that led here
	Depth     int
	Priority  float64 // lower = higher priority (BFS: priority == depth)
}

// PageData holds the result of fetching and parsing a single page.
type PageData struct {
	URL       string
	OriginURL string
	Depth     int
	Title     string
	Body      string   // visible text, stripped of tags
	Links     []string // absolute URLs extracted from <a href>
	FetchedAt time.Time
}

// --- Job system types ---

// JobParams holds the user-provided parameters for creating a crawl job.
type JobParams struct {
	OriginURL    string `json:"origin_url"`
	MaxDepth     int    `json:"max_depth"`
	RateLimit    int    `json:"rate_limit"`     // requests per second per domain
	MaxQueueSize int    `json:"max_queue_size"` // back-pressure threshold
}

// Job represents a crawler job's metadata (stored in SQLite).
type Job struct {
	ID           string     `json:"id"`
	OriginURL    string     `json:"origin_url"`
	MaxDepth     int        `json:"max_depth"`
	RateLimit    int        `json:"rate_limit"`
	MaxQueueSize int        `json:"max_queue_size"`
	Status       string     `json:"status"` // pending, running, completed, interrupted, failed
	PagesCrawled int        `json:"pages_crawled"`
	PagesFailed  int        `json:"pages_failed"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// JobState is the full state persisted to [crawlerId].data.
type JobState struct {
	ID           string      `json:"id"`
	OriginURL    string      `json:"origin_url"`
	MaxDepth     int         `json:"max_depth"`
	RateLimit    int         `json:"rate_limit"`
	MaxQueueSize int         `json:"max_queue_size"`
	Status       string      `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty"`
	PagesCrawled int         `json:"pages_crawled"`
	PagesFailed  int         `json:"pages_failed"`
	PendingURLs  []QueueItem `json:"pending_urls"`
	Logs         []LogEntry  `json:"logs"`
}

// QueueItem is a URL waiting to be fetched within a job.
type QueueItem struct {
	URL   string `json:"url"`
	Depth int    `json:"depth"`
}

// LogEntry is a single log line in a job's state.
type LogEntry struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"` // info, error, warn
	Msg   string    `json:"msg"`
}

// WordEntry is a single record in a [letter].data file.
type WordEntry struct {
	Word   string `json:"word"`
	URL    string `json:"url"`
	Origin string `json:"origin"`
	Depth  int    `json:"depth"`
	Freq   int    `json:"freq"`
}

// SearchResult is a single result returned to the search page.
type SearchResult struct {
	URL            string  `json:"url"`
	OriginURL      string  `json:"origin_url"`
	Depth          int     `json:"depth"`
	Freq           int     `json:"freq"`
	RelevanceScore float64 `json:"relevance_score"`
}
