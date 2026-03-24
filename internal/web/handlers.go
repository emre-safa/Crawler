package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/emre-safa/crawler/internal/types"
)

// --- Crawler page (job list + create form) ---

type crawlerPageData struct {
	Jobs  []types.Job
	Error string
}

func (s *Server) handleCrawlerPage(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.jobManager.List()
	if err != nil {
		log.Printf("web: list jobs: %v", err)
	}
	s.render(w, "crawler.html", crawlerPageData{Jobs: jobs})
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, "crawler.html", crawlerPageData{Error: "Invalid form data"})
		return
	}

	params := types.JobParams{
		OriginURL: r.FormValue("origin_url"),
	}
	if v, err := strconv.Atoi(r.FormValue("max_depth")); err == nil {
		params.MaxDepth = v
	}
	if v, err := strconv.Atoi(r.FormValue("rate_limit")); err == nil {
		params.RateLimit = v
	}
	if v, err := strconv.Atoi(r.FormValue("max_queue_size")); err == nil {
		params.MaxQueueSize = v
	}

	job, err := s.jobManager.Create(params)
	if err != nil {
		jobs, _ := s.jobManager.List()
		s.render(w, "crawler.html", crawlerPageData{Jobs: jobs, Error: err.Error()})
		return
	}

	http.Redirect(w, r, "/status/"+job.ID, http.StatusSeeOther)
}

// --- Status page ---

type statusPageData struct {
	State *types.JobState
}

func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	crawlerID := r.PathValue("crawlerId")
	state, err := s.jobManager.GetJobState(crawlerID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	s.render(w, "status.html", statusPageData{State: state})
}

// --- Long polling endpoint ---

type pollResponse struct {
	Status       string           `json:"status"`
	Logs         []types.LogEntry `json:"logs"`
	PagesCrawled int              `json:"pages_crawled"`
	PagesFailed  int              `json:"pages_failed"`
	PendingCount int              `json:"pending_count"`
	LogIndex     int              `json:"log_index"`
}

func (s *Server) handleStatusPoll(w http.ResponseWriter, r *http.Request) {
	crawlerID := r.PathValue("crawlerId")
	afterStr := r.URL.Query().Get("after")
	after, _ := strconv.Atoi(afterStr)

	ctx := r.Context()
	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, err := s.jobManager.GetJobState(crawlerID)
		if err != nil {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}

		if len(state.Logs) > after || state.Status != "running" {
			var newLogs []types.LogEntry
			if after < len(state.Logs) {
				newLogs = state.Logs[after:]
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pollResponse{
				Status:       state.Status,
				Logs:         newLogs,
				PagesCrawled: state.PagesCrawled,
				PagesFailed:  state.PagesFailed,
				PendingCount: len(state.PendingURLs),
				LogIndex:     len(state.Logs),
			})
			return
		}

		select {
		case <-ticker.C:
			continue
		case <-deadline:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pollResponse{
				Status:   state.Status,
				LogIndex: after,
			})
			return
		case <-ctx.Done():
			return
		}
	}
}

// --- Cancel job ---

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	crawlerID := r.PathValue("crawlerId")
	if err := s.jobManager.Cancel(crawlerID); err != nil {
		log.Printf("web: cancel job %s: %v", crawlerID, err)
	}
	// Give it a moment to process the cancellation
	time.Sleep(200 * time.Millisecond)
	http.Redirect(w, r, "/status/"+crawlerID, http.StatusSeeOther)
}

// --- Search page ---

type searchPageData struct {
	Query        string
	Results      []types.SearchResult
	TotalResults int
	Page         int
	Offset       int
	HasPrev      bool
	HasNext      bool
	PrevPage     int
	NextPage     int
}

func (s *Server) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("p")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	const perPage = 20

	if query == "" {
		s.render(w, "search.html", searchPageData{Page: 1})
		return
	}

	results, total := searchWords(s.wordStore, query, page, perPage, "relevance")

	s.render(w, "search.html", searchPageData{
		Query:        query,
		Results:      results,
		TotalResults: total,
		Page:         page,
		Offset:       (page-1)*perPage + 1,
		HasPrev:      page > 1,
		HasNext:      page*perPage < total,
		PrevPage:     page - 1,
		NextPage:     page + 1,
	})
}

// --- JSON search API ---

type apiSearchResponse struct {
	Query        string               `json:"query"`
	TotalResults int                  `json:"total_results"`
	Results      []types.SearchResult `json:"results"`
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	sortBy := r.URL.Query().Get("sortBy")
	if sortBy == "" {
		sortBy = "relevance"
	}

	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiSearchResponse{Query: "", Results: []types.SearchResult{}})
		return
	}

	results, total := searchWords(s.wordStore, query, 1, 10000, sortBy)
	if results == nil {
		results = []types.SearchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiSearchResponse{
		Query:        query,
		TotalResults: total,
		Results:      results,
	})
}

// --- Template rendering ---

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	t, ok := s.templates[name]
	if !ok {
		log.Printf("web: unknown template %s", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("web: render %s: %v", name, err)
	}
}
