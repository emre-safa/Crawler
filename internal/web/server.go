package web

import (
	"html/template"
	"net/http"

	"github.com/emre-safa/crawler/internal/filestorage"
	"github.com/emre-safa/crawler/internal/job"
)

// Server holds the HTTP server state and dependencies.
type Server struct {
	templates  map[string]*template.Template
	jobManager *job.Manager
	wordStore  *filestorage.WordStore
	mux        *http.ServeMux
}

// NewServer creates a Server with all routes registered.
func NewServer(tmpl map[string]*template.Template, jm *job.Manager, ws *filestorage.WordStore) *Server {
	s := &Server{
		templates:  tmpl,
		jobManager: jm,
		wordStore:  ws,
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleCrawlerPage)
	s.mux.HandleFunc("POST /{$}", s.handleCreateJob)
	s.mux.HandleFunc("GET /status/{crawlerId}", s.handleStatusPage)
	s.mux.HandleFunc("GET /status/{crawlerId}/poll", s.handleStatusPoll)
	s.mux.HandleFunc("POST /status/{crawlerId}/cancel", s.handleCancelJob)
	s.mux.HandleFunc("GET /search", s.handleSearchPage)
	s.mux.HandleFunc("GET /api/search", s.handleAPISearch)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
