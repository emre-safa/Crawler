package filestorage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unicode"

	"github.com/emre-safa/crawler/internal/types"
)

const numBuckets = 36 // a-z (0-25) + 0-9 (26-35)

// WordStore manages the [letter].data files in the storage directory.
// Each file holds NDJSON entries for words starting with that letter/digit.
type WordStore struct {
	locks   [numBuckets]sync.RWMutex
	dataDir string // path to storage/ directory
}

// NewWordStore creates a WordStore backed by the given directory.
func NewWordStore(dataDir string) (*WordStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("filestorage: create storage dir: %w", err)
	}
	return &WordStore{dataDir: dataDir}, nil
}

// Append writes a word entry to the appropriate [letter].data file.
func (ws *WordStore) Append(entry types.WordEntry) error {
	idx := bucketIndex(entry.Word)
	if idx < 0 {
		return nil // non-alphanumeric first character, skip
	}

	ws.locks[idx].Lock()
	defer ws.locks[idx].Unlock()

	filename := bucketFilename(idx)
	path := filepath.Join(ws.dataDir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("filestorage: open %s: %w", filename, err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// Lookup reads all entries for a given word from its [letter].data file.
func (ws *WordStore) Lookup(word string) ([]types.WordEntry, error) {
	idx := bucketIndex(word)
	if idx < 0 {
		return nil, nil
	}

	ws.locks[idx].RLock()
	defer ws.locks[idx].RUnlock()

	filename := bucketFilename(idx)
	path := filepath.Join(ws.dataDir, filename)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var results []types.WordEntry
	scanner := bufio.NewScanner(f)
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var entry types.WordEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		if entry.Word == word {
			results = append(results, entry)
		}
	}

	return results, scanner.Err()
}

// bucketIndex returns 0-25 for a-z, 26-35 for 0-9, or -1 for other.
func bucketIndex(word string) int {
	if len(word) == 0 {
		return -1
	}
	ch := rune(word[0])
	if unicode.IsLetter(ch) {
		ch = unicode.ToLower(ch)
		if ch >= 'a' && ch <= 'z' {
			return int(ch - 'a')
		}
		return -1
	}
	if ch >= '0' && ch <= '9' {
		return 26 + int(ch-'0')
	}
	return -1
}

// bucketFilename returns the .data filename for a bucket index.
func bucketFilename(idx int) string {
	if idx < 26 {
		return string(rune('a'+idx)) + ".data"
	}
	return string(rune('0'+idx-26)) + ".data"
}
