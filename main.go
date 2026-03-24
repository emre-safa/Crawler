package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/emre-safa/crawler/internal/config"
	"github.com/emre-safa/crawler/internal/filestorage"
	"github.com/emre-safa/crawler/internal/job"
	"github.com/emre-safa/crawler/internal/store"
	"github.com/emre-safa/crawler/internal/web"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	// Parse flags
	addr := flag.String("addr", ":3600", "HTTP listen address")
	dbPath := flag.String("db", "crawler.db", "Path to SQLite database")
	dataDir := flag.String("data", "data", "Directory for .data files")
	flag.Parse()

	cfg := config.Default()
	cfg.ListenAddr = *addr
	cfg.DBPath = *dbPath
	cfg.DataDir = *dataDir

	// Create data directories
	for _, dir := range []string{
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "jobs"),
		filepath.Join(cfg.DataDir, "storage"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Open SQLite
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer st.Close()

	// Initialize file storage
	storageDir := filepath.Join(cfg.DataDir, "storage")
	words, err := filestorage.NewWordStore(storageDir)
	if err != nil {
		log.Fatalf("Failed to init word store: %v", err)
	}

	// Parse templates: clone layout per page so each gets its own "content" block
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	layoutTmpl := template.Must(template.New("layout").Funcs(funcMap).ParseFiles("templates/layout.html"))

	pages := []string{"crawler.html", "status.html", "search.html"}
	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t := template.Must(layoutTmpl.Clone())
		template.Must(t.ParseFiles("templates/" + page))
		templates[page] = t
	}

	// Create job manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jm := job.NewManager(ctx, cfg, st, words)

	// Create web server
	srv := web.NewServer(templates, jm, words)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv,
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Signal received, shutting down...")
		cancel()
		jm.Shutdown()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	// Start server
	log.Printf("Starting server on %s", cfg.ListenAddr)
	fmt.Printf("🕷  Crawler running at http://localhost%s\n", cfg.ListenAddr)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	_ = ctx // used by signal handler
	log.Println("Shutdown complete.")
}
