package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/emre-safa/crawler/internal/types"
)

// Store wraps a SQLite database for job metadata.
type Store struct {
	db *sql.DB
}

// Open creates or opens a SQLite database and ensures the schema exists.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: schema init: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateJob inserts a new job record.
func (s *Store) CreateJob(job *types.Job) error {
	_, err := s.db.Exec(
		`INSERT INTO jobs (id, origin_url, max_depth, rate_limit, max_queue_size, status, pages_crawled, pages_failed, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.OriginURL, job.MaxDepth, job.RateLimit, job.MaxQueueSize,
		job.Status, job.PagesCrawled, job.PagesFailed,
		job.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// UpdateJob updates a job's mutable fields.
func (s *Store) UpdateJob(job *types.Job) error {
	var finishedAt *string
	if job.FinishedAt != nil {
		t := job.FinishedAt.Format(time.RFC3339)
		finishedAt = &t
	}
	_, err := s.db.Exec(
		`UPDATE jobs SET status=?, pages_crawled=?, pages_failed=?, finished_at=?, error=? WHERE id=?`,
		job.Status, job.PagesCrawled, job.PagesFailed, finishedAt, job.Error, job.ID,
	)
	return err
}

// GetJob retrieves a single job by ID.
func (s *Store) GetJob(id string) (*types.Job, error) {
	row := s.db.QueryRow(
		`SELECT id, origin_url, max_depth, rate_limit, max_queue_size, status, pages_crawled, pages_failed, created_at, finished_at, error
		 FROM jobs WHERE id=?`, id,
	)
	return scanJob(row)
}

// ListJobs returns all jobs ordered by creation time (newest first).
func (s *Store) ListJobs() ([]types.Job, error) {
	rows, err := s.db.Query(
		`SELECT id, origin_url, max_depth, rate_limit, max_queue_size, status, pages_crawled, pages_failed, created_at, finished_at, error
		 FROM jobs ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []types.Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// --- helpers ---

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanJob(row *sql.Row) (*types.Job, error) {
	var j types.Job
	var createdAt string
	var finishedAt, errStr sql.NullString

	err := row.Scan(&j.ID, &j.OriginURL, &j.MaxDepth, &j.RateLimit, &j.MaxQueueSize,
		&j.Status, &j.PagesCrawled, &j.PagesFailed, &createdAt, &finishedAt, &errStr)
	if err != nil {
		return nil, err
	}

	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		j.FinishedAt = &t
	}
	if errStr.Valid {
		j.Error = errStr.String
	}
	return &j, nil
}

func scanJobRow(rows *sql.Rows) (*types.Job, error) {
	var j types.Job
	var createdAt string
	var finishedAt, errStr sql.NullString

	err := rows.Scan(&j.ID, &j.OriginURL, &j.MaxDepth, &j.RateLimit, &j.MaxQueueSize,
		&j.Status, &j.PagesCrawled, &j.PagesFailed, &createdAt, &finishedAt, &errStr)
	if err != nil {
		return nil, err
	}

	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		j.FinishedAt = &t
	}
	if errStr.Valid {
		j.Error = errStr.String
	}
	return &j, nil
}
