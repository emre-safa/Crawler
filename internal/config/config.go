package config

import "time"

// Config holds server-level configuration.
type Config struct {
	ListenAddr   string        // e.g. ":8080"
	DBPath       string        // path to SQLite database
	DataDir      string        // root directory for .data files
	FetchTimeout time.Duration // per-request timeout
	MaxRedirects int
	MaxBodySize  int64 // bytes
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		ListenAddr:   ":3600",
		DBPath:       "crawler.db",
		DataDir:      "data",
		FetchTimeout: 10 * time.Second,
		MaxRedirects: 5,
		MaxBodySize:  5 * 1024 * 1024, // 5 MB
	}
}
