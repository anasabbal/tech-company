package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

type CompanyEntry struct {
	Company     string    `json:"company"`
	CountryFlag string    `json:"country_flag"`
	Country     string    `json:"country"`
	Jobs        []JobItem `json:"jobs"`
}

type JobItem struct {
	Company     string `json:"company"`
	JobTitle    string `json:"job_title"`
	Location    string `json:"location"`
	Country     string `json:"country"`
	CountryFlag string `json:"country_flag"`
	ApplyURL    string `json:"apply_url"`
	Added       string `json:"added"`
}

type JobView struct {
	ID          string `json:"id"`
	Company     string `json:"company"`
	JobTitle    string `json:"job_title"`
	Location    string `json:"location"`
	Country     string `json:"country"`
	CountryFlag string `json:"country_flag"`
	ApplyURL    string `json:"apply_url"`
	Added       string `json:"added"`
}

type JobsResponse struct {
	Companies      []CompanyEntry `json:"companies"`
	Jobs           []JobView      `json:"jobs"`
	Countries      []string       `json:"countries"`
	Stats          map[string]int `json:"stats"`
	LastUpdatedISO string         `json:"last_updated_iso"`
}

func main() {
	payload, err := loadJobs("job.json")
	if err != nil {
		log.Fatalf("failed to load jobs: %v", err)
	}

	mux := http.NewServeMux()
	staticFiles, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("failed to prepare embedded assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(staticFiles))

	mux.Handle("/web/", http.StripPrefix("/web/", fileServer))
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(payload)
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
	log.Printf("jobs UI available at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func port() string {
	if value := os.Getenv("PORT"); strings.TrimSpace(value) != "" {
		return value
	}
	return "8080"
}

func loadJobs(path string) (*JobsResponse, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var companies []CompanyEntry
	if err := json.Unmarshal(data, &companies); err != nil {
		return nil, err
	}

	jobs := make([]JobView, 0)
	countrySet := make(map[string]struct{})

	for companyIndex, company := range companies {
		if company.Country != "" {
			countrySet[company.Country] = struct{}{}
		}

		for jobIndex, job := range company.Jobs {
			if job.Country != "" {
				countrySet[job.Country] = struct{}{}
			}

			jobs = append(jobs, JobView{
				ID:          buildJobID(company.Company, companyIndex, jobIndex),
				Company:     job.Company,
				JobTitle:    job.JobTitle,
				Location:    job.Location,
				Country:     job.Country,
				CountryFlag: job.CountryFlag,
				ApplyURL:    job.ApplyURL,
				Added:       job.Added,
			})
		}
	}

	sort.Slice(jobs, func(i, j int) bool {
		return parseAdded(jobs[i].Added).After(parseAdded(jobs[j].Added))
	})

	countries := make([]string, 0, len(countrySet))
	for country := range countrySet {
		countries = append(countries, country)
	}
	sort.Strings(countries)

	return &JobsResponse{
		Companies: companies,
		Jobs:      jobs,
		Countries: countries,
		Stats: map[string]int{
			"companies": len(companies),
			"jobs":      len(jobs),
			"countries": len(countries),
		},
		LastUpdatedISO: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func buildJobID(company string, companyIndex, jobIndex int) string {
	slug := strings.ToLower(company)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	return slug + "-" + strconvItoa(companyIndex) + "-" + strconvItoa(jobIndex)
}

func parseAdded(value string) time.Time {
	parsed, err := time.Parse("January 2, 2006", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func strconvItoa(v int) string {
	return strconv.Itoa(v)
}
