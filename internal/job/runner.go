package job

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/emre-safa/crawler/internal/filestorage"
	"github.com/emre-safa/crawler/internal/frontier"
	"github.com/emre-safa/crawler/internal/index"
	"github.com/emre-safa/crawler/internal/parser"
	"github.com/emre-safa/crawler/internal/types"
)

// run executes the crawl loop for a single job. Called as a goroutine.
func (m *Manager) run(ctx context.Context, rj *runningJob) {
	jobsDir := filepath.Join(m.dataDir, "jobs")

	// Seed the queue
	rj.mu.RLock()
	queue := make([]types.QueueItem, len(rj.state.PendingURLs))
	copy(queue, rj.state.PendingURLs)
	rj.mu.RUnlock()

	lastFlush := time.Now()
	pagesSinceFlush := 0

	for len(queue) > 0 {
		// Check cancellation
		select {
		case <-ctx.Done():
			rj.addLog("warn", "Job interrupted by user")
			rj.setStatus("interrupted")
			rj.setPendingURLs(queue)
			filestorage.WriteJobState(jobsDir, rj.state)
			return
		default:
		}

		// Pop next item
		item := queue[0]
		queue = queue[1:]

		// Canonicalize
		canonical := frontier.Canonicalize(item.URL)
		if canonical == "" {
			continue
		}

		// Check per-job visited set (atomic check-and-mark)
		if !rj.visited.MarkVisited(canonical) {
			continue // already visited
		}

		rj.addLog("info", fmt.Sprintf("Fetching %s (depth %d)", canonical, item.Depth))

		// Fetch
		crawlItem := types.CrawlItem{
			URL:       canonical,
			OriginURL: rj.params.OriginURL,
			Depth:     item.Depth,
		}
		body, status, err := m.fetch.Fetch(ctx, crawlItem, rj.params.RateLimit)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled during fetch
				rj.addLog("warn", "Job interrupted by user")
				rj.setStatus("interrupted")
				rj.setPendingURLs(queue)
				filestorage.WriteJobState(jobsDir, rj.state)
				return
			}
			rj.addLog("error", fmt.Sprintf("Fetch %s: %v", canonical, err))
			rj.incFailed()
			// If origin page fails, stop the whole job
			if item.Depth == 0 {
				rj.addLog("error", "Origin page failed, stopping job")
				rj.setStatus("failed")
				filestorage.WriteJobState(jobsDir, rj.state)
				return
			}
			continue
		}

		if status < 200 || status >= 300 {
			rj.addLog("error", fmt.Sprintf("Fetch %s: HTTP %d", canonical, status))
			rj.incFailed()
			if item.Depth == 0 {
				rj.addLog("error", fmt.Sprintf("Origin page returned HTTP %d, stopping job", status))
				rj.setStatus("failed")
				filestorage.WriteJobState(jobsDir, rj.state)
				return
			}
			continue
		}

		// Parse HTML
		page := parser.Parse(body, crawlItem)
		rj.addLog("info", fmt.Sprintf("Parsed %s: %q (%d links found)", canonical, page.Title, len(page.Links)))

		// Tokenize and count word frequencies
		bodyTokens := index.Tokenize(page.Body)
		titleTokens := index.Tokenize(page.Title)
		freq := make(map[string]int)
		for _, t := range bodyTokens {
			freq[t]++
		}
		for _, t := range titleTokens {
			freq[t]++
		}

		// Store each word in [letter].data
		for word, count := range freq {
			entry := types.WordEntry{
				Word:   word,
				URL:    canonical,
				Origin: rj.params.OriginURL,
				Depth:  item.Depth,
				Freq:   count,
			}
			if err := m.words.Append(entry); err != nil {
				rj.addLog("error", fmt.Sprintf("Store word %q: %v", word, err))
			}
		}

		rj.incCrawled()
		pagesSinceFlush++

		// Enqueue child links
		if item.Depth+1 <= rj.params.MaxDepth {
			for _, link := range page.Links {
				cLink := frontier.Canonicalize(link)
				if cLink != "" && !rj.visited.IsVisited(cLink) {
					// Back-pressure: respect max queue size
					if len(queue) < rj.params.MaxQueueSize {
						queue = append(queue, types.QueueItem{URL: cLink, Depth: item.Depth + 1})
					}
				}
			}
		}

		// Periodically flush state to disk (every 10 pages or every 5 seconds)
		if pagesSinceFlush >= 10 || time.Since(lastFlush) > 5*time.Second {
			rj.setPendingURLs(queue)
			rj.mu.RLock()
			filestorage.WriteJobState(jobsDir, rj.state)
			rj.mu.RUnlock()
			lastFlush = time.Now()
			pagesSinceFlush = 0
		}
	}

	rj.addLog("info", "Crawl completed successfully")
	rj.setStatus("completed")
	rj.setPendingURLs(nil)
	filestorage.WriteJobState(jobsDir, rj.state)
}
