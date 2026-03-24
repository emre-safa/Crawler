# Crawler

A web crawler and search engine built from scratch in Go. No high-level scraping libraries — only `net/http` for fetching and `golang.org/x/net/html` for parsing.

## Features

- **Multiple concurrent crawl jobs** — start, monitor, and cancel jobs from a web dashboard
- **Live status updates** — long-polling delivers real-time logs and stats to the browser
- **Full-text search** — file-based word index with relevance scoring and pagination
- **JSON search API** — `GET /api/search?query=<word>&sortBy=relevance`
- **Hybrid storage** — SQLite for job metadata, flat NDJSON files for word index
- **Per-job isolation** — each job has its own visited URL set, so re-crawling works

## Architecture

```
main.go                      Entry point, flags, server startup
internal/
  config/config.go           Server configuration
  fetcher/fetcher.go         HTTP fetcher with per-domain rate limiting
  parser/parser.go           HTML parser (title, body text, links)
  frontier/frontier.go       URL canonicalization
  index/index.go             Tokenizer + stop word filter
  filestorage/
    words.go                 Per-letter NDJSON word storage (a.data … z.data)
    visited.go               Per-job visited URL set
    jobstate.go              Atomic job state persistence
  store/
    schema.go                SQLite schema (jobs table, WAL mode)
    sqlite.go                Job CRUD operations
  job/
    manager.go               Job lifecycle: create, cancel, shutdown
    runner.go                Crawl loop: fetch → parse → tokenize → store → enqueue
  web/
    server.go                HTTP routes
    handlers.go              Page handlers + JSON API + long polling
    search.go                Search logic with relevance scoring
templates/                   Server-side rendered HTML (Go templates)
```

### How crawling works

1. User submits an origin URL, max depth, rate limit, and queue capacity
2. A new job gets a unique ID (`[epoch]_[hex]`) and starts in its own goroutine
3. The runner does BFS: canonicalize URL → check visited → fetch (with rate limiting) → parse HTML → tokenize text → append words to `[letter].data` → enqueue child links
4. State is flushed to disk every 10 pages or 5 seconds
5. The status page polls for updates via long polling (15s timeout, 500ms interval)

### How search works

Each word is stored in `[first_letter].data` as NDJSON with URL, origin, depth, and frequency. Search tokenizes the query, looks up each token's file, intersects results (AND semantics), and ranks by:

```
relevance_score = (frequency × 10) + 1000 − (depth × 5)
```

## Getting started

```bash
# Clone and run
git clone https://github.com/emre-safa/Crawler.git
cd Crawler
go run . --addr :3600

# Open the dashboard
# http://localhost:3600

# Search via API
curl "http://localhost:3600/api/search?query=example&sortBy=relevance"
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3600` | HTTP listen address |
| `-db` | `crawler.db` | SQLite database path |
| `-data` | `data` | Directory for `.data` files |

## Dependencies

- `golang.org/x/net/html` — HTML tokenizer
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGo)

## Data files

All runtime data is generated automatically on first run:

- `crawler.db` — SQLite job metadata
- `data/storage/[letter].data` — NDJSON word index (one file per first letter)
- `data/jobs/[id]/state.data` — per-job state
- `data/jobs/[id]/visited_urls.data` — per-job visited URLs

These are gitignored and not required to clone and run.
