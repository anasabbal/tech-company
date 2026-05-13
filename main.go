package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"test/handler"
	"test/store"
)

//go:embed web/*
var webFS embed.FS

func main() {
	if err := loadDotEnv(".env"); err != nil {
		log.Printf("skipping .env: %v", err)
	}

	jobStore, err := store.NewJobStore("job.json")
	if err != nil {
		log.Fatalf("failed to load job.json: %v", err)
	}
	listings := handler.NewListingsHandler(jobStore)
	refreshInterval := autoRefreshInterval()
	if refreshInterval > 0 {
		go jobStore.StartAutoRefresh(context.Background(), refreshInterval)
		log.Printf("job source auto-refresh enabled every %s", refreshInterval)
	}

	mux := http.NewServeMux()

	staticFiles, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("failed to prepare embedded assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(staticFiles))
	mux.Handle("/web/", http.StripPrefix("/web/", fileServer))

	mux.HandleFunc("/api/listings", listings.GetAll)
	mux.HandleFunc("/api/listings/filter", listings.Filter)
	mux.HandleFunc("/api/listings/stats", listings.GetStats)
	mux.HandleFunc("/api/listings/countries", listings.GetCountries)
	mux.HandleFunc("/api/listings/refresh", listings.Refresh)

	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"companies":        jobStore.GetAllCompanies(),
			"jobs":             jobStore.GetAllJobs(),
			"countries":        jobStore.GetCountries(),
			"stats":            jobStore.Stats(),
			"last_updated_iso": time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		indexHTML, err := fs.ReadFile(staticFiles, "index.html")
		if err != nil {
			http.Error(w, "failed to load page", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	addr := ":" + port()
	log.Printf("server listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func port() string {
	if v := os.Getenv("PORT"); strings.TrimSpace(v) != "" {
		return v
	}
	return "8080"
}

func autoRefreshInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("JOB_AUTO_REFRESH_MINUTES"))
	if v == "" {
		return 0
	}
	minutes, err := time.ParseDuration(v + "m")
	if err != nil || minutes <= 0 {
		log.Printf("ignoring invalid JOB_AUTO_REFRESH_MINUTES=%q", v)
		return 0
	}
	return minutes
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}

		// Keep shell-provided env vars as the highest-priority source.
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}

	return nil
}
