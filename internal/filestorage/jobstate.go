package filestorage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/emre-safa/crawler/internal/types"
)

// JobDir returns the directory path for a given crawler job.
func JobDir(jobsDir, crawlerID string) string {
	return filepath.Join(jobsDir, crawlerID)
}

// WriteJobState writes the full job state to [crawlerId]/state.data as JSON.
func WriteJobState(jobsDir string, state *types.JobState) error {
	dir := JobDir(jobsDir, state.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("filestorage: create job dir: %w", err)
	}

	path := filepath.Join(dir, "state.data")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("filestorage: marshal job state: %w", err)
	}

	// Write atomically: write to temp file, then rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("filestorage: write job state: %w", err)
	}
	return os.Rename(tmp, path)
}

// ReadJobState reads a job state from [crawlerId]/state.data.
func ReadJobState(jobsDir, crawlerID string) (*types.JobState, error) {
	path := filepath.Join(JobDir(jobsDir, crawlerID), "state.data")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("filestorage: read job state %s: %w", crawlerID, err)
	}

	var state types.JobState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("filestorage: parse job state %s: %w", crawlerID, err)
	}
	return &state, nil
}
