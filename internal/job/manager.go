package job

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emre-safa/crawler/internal/config"
	"github.com/emre-safa/crawler/internal/fetcher"
	"github.com/emre-safa/crawler/internal/filestorage"
	"github.com/emre-safa/crawler/internal/store"
	"github.com/emre-safa/crawler/internal/types"
)

// runningJob tracks a job that is currently executing.
type runningJob struct {
	cancel  context.CancelFunc
	mu      sync.RWMutex
	state   *types.JobState
	params  types.JobParams
	visited *filestorage.VisitedSet
}

func (rj *runningJob) addLog(level, msg string) {
	rj.mu.Lock()
	defer rj.mu.Unlock()
	rj.state.Logs = append(rj.state.Logs, types.LogEntry{
		Time:  time.Now(),
		Level: level,
		Msg:   msg,
	})
}

func (rj *runningJob) incCrawled() {
	rj.mu.Lock()
	rj.state.PagesCrawled++
	rj.mu.Unlock()
}

func (rj *runningJob) incFailed() {
	rj.mu.Lock()
	rj.state.PagesFailed++
	rj.mu.Unlock()
}

func (rj *runningJob) setStatus(status string) {
	rj.mu.Lock()
	rj.state.Status = status
	if status == "completed" || status == "interrupted" || status == "failed" {
		now := time.Now()
		rj.state.FinishedAt = &now
	}
	rj.mu.Unlock()
}

func (rj *runningJob) setPendingURLs(urls []types.QueueItem) {
	rj.mu.Lock()
	rj.state.PendingURLs = urls
	rj.mu.Unlock()
}

// Manager orchestrates crawler job creation, cancellation, and lifecycle.
type Manager struct {
	ctx     context.Context
	mu      sync.Mutex
	store   *store.Store
	words   *filestorage.WordStore
	fetch   *fetcher.Fetcher
	jobs    map[string]*runningJob
	wg      sync.WaitGroup
	dataDir string
	cfg     *config.Config
}

// NewManager creates a job Manager.
func NewManager(ctx context.Context, cfg *config.Config, st *store.Store, words *filestorage.WordStore) *Manager {
	fetchCfg := fetcher.Config{
		FetchTimeout: cfg.FetchTimeout,
		MaxRedirects: cfg.MaxRedirects,
		MaxBodySize:  cfg.MaxBodySize,
		DefaultRate:  2,
	}
	return &Manager{
		ctx:     ctx,
		store:   st,
		words:   words,
		fetch:   fetcher.New(fetchCfg),
		jobs:    make(map[string]*runningJob),
		dataDir: cfg.DataDir,
		cfg:     cfg,
	}
}

// Create validates params, generates a crawler ID, persists the job, and starts crawling.
func (m *Manager) Create(params types.JobParams) (*types.Job, error) {
	// Validate origin URL
	params.OriginURL = strings.TrimSpace(params.OriginURL)
	u, err := url.Parse(params.OriginURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid origin URL: must be a valid HTTP(S) URL")
	}

	// Apply defaults
	if params.MaxDepth <= 0 {
		params.MaxDepth = 3
	}
	if params.RateLimit <= 0 {
		params.RateLimit = 2
	}
	if params.MaxQueueSize <= 0 {
		params.MaxQueueSize = 10000
	}

	// Generate crawler ID: [EpochTimeCreated]_[RandomHex6]
	hexBytes := make([]byte, 3)
	rand.Read(hexBytes)
	id := fmt.Sprintf("%d_%x", time.Now().Unix(), hexBytes)

	now := time.Now()
	job := &types.Job{
		ID:           id,
		OriginURL:    params.OriginURL,
		MaxDepth:     params.MaxDepth,
		RateLimit:    params.RateLimit,
		MaxQueueSize: params.MaxQueueSize,
		Status:       "running",
		CreatedAt:    now,
	}

	// Persist to SQLite
	if err := m.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	// Create initial job state file
	state := &types.JobState{
		ID:           id,
		OriginURL:    params.OriginURL,
		MaxDepth:     params.MaxDepth,
		RateLimit:    params.RateLimit,
		MaxQueueSize: params.MaxQueueSize,
		Status:       "running",
		CreatedAt:    now,
		PendingURLs:  []types.QueueItem{{URL: params.OriginURL, Depth: 0}},
		Logs: []types.LogEntry{{
			Time:  now,
			Level: "info",
			Msg:   fmt.Sprintf("Job created. Crawling %s up to depth %d", params.OriginURL, params.MaxDepth),
		}},
	}
	jobsDir := filepath.Join(m.dataDir, "jobs")
	if err := filestorage.WriteJobState(jobsDir, state); err != nil {
		return nil, fmt.Errorf("write initial job state: %w", err)
	}

	// Create per-job visited set
	visitedPath := filepath.Join(filestorage.JobDir(jobsDir, id), "visited_urls.data")
	visited, err := filestorage.NewVisitedSet(visitedPath)
	if err != nil {
		return nil, fmt.Errorf("create visited set: %w", err)
	}

	// Start the crawl goroutine
	jobCtx, cancel := context.WithCancel(m.ctx)
	rj := &runningJob{
		cancel:  cancel,
		state:   state,
		params:  params,
		visited: visited,
	}

	m.mu.Lock()
	m.jobs[id] = rj
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer rj.visited.Close()
		m.run(jobCtx, rj)
		// Update SQLite with final status
		rj.mu.RLock()
		job.Status = rj.state.Status
		job.PagesCrawled = rj.state.PagesCrawled
		job.PagesFailed = rj.state.PagesFailed
		job.FinishedAt = rj.state.FinishedAt
		rj.mu.RUnlock()
		m.store.UpdateJob(job)
		m.mu.Lock()
		delete(m.jobs, id)
		m.mu.Unlock()
	}()

	return job, nil
}

// Cancel stops a running job.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	rj, ok := m.jobs[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("job %s is not running", id)
	}

	rj.cancel()
	return nil
}

// List returns all jobs from SQLite.
func (m *Manager) List() ([]types.Job, error) {
	return m.store.ListJobs()
}

// GetJob returns a job from SQLite.
func (m *Manager) GetJob(id string) (*types.Job, error) {
	return m.store.GetJob(id)
}

// GetJobState returns the current in-memory state for a running job,
// or reads from the .data file for completed jobs.
func (m *Manager) GetJobState(id string) (*types.JobState, error) {
	m.mu.Lock()
	rj, ok := m.jobs[id]
	m.mu.Unlock()

	if ok {
		rj.mu.RLock()
		defer rj.mu.RUnlock()
		cp := *rj.state
		cp.Logs = make([]types.LogEntry, len(rj.state.Logs))
		copy(cp.Logs, rj.state.Logs)
		cp.PendingURLs = make([]types.QueueItem, len(rj.state.PendingURLs))
		copy(cp.PendingURLs, rj.state.PendingURLs)
		return &cp, nil
	}

	jobsDir := filepath.Join(m.dataDir, "jobs")
	return filestorage.ReadJobState(jobsDir, id)
}

// IsRunning returns true if the job is currently active.
func (m *Manager) IsRunning(id string) bool {
	m.mu.Lock()
	_, ok := m.jobs[id]
	m.mu.Unlock()
	return ok
}

// Shutdown cancels all running jobs and waits for them to finish.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	for _, rj := range m.jobs {
		rj.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
