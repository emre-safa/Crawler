package filestorage

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// VisitedSet manages the global visited_urls.data file.
// It keeps an in-memory set for fast lookups and appends to the file for durability.
type VisitedSet struct {
	mu   sync.RWMutex
	set  map[string]bool
	file *os.File
	path string
}

// NewVisitedSet loads or creates the visited URLs file.
func NewVisitedSet(path string) (*VisitedSet, error) {
	vs := &VisitedSet{
		set:  make(map[string]bool),
		path: path,
	}

	// Read existing file if it exists
	if data, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				vs.set[line] = true
			}
		}
		data.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("filestorage: read visited: %w", err)
		}
	}

	// Open file for appending
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("filestorage: open visited for append: %w", err)
	}
	vs.file = f

	return vs, nil
}

// IsVisited returns true if the URL has been seen.
func (vs *VisitedSet) IsVisited(url string) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.set[url]
}

// MarkVisited atomically checks and marks a URL as visited.
// Returns true if the URL was newly added, false if already visited.
func (vs *VisitedSet) MarkVisited(url string) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.set[url] {
		return false
	}

	vs.set[url] = true
	fmt.Fprintln(vs.file, url)
	return true
}

// Count returns the number of visited URLs.
func (vs *VisitedSet) Count() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return len(vs.set)
}

// Close flushes and closes the file.
func (vs *VisitedSet) Close() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return vs.file.Close()
}
