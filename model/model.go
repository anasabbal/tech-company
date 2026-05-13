package model

import "time"

// ---------------------------------------------------------------------------
// Existing concurrency-demo types (unchanged)
// ---------------------------------------------------------------------------

// Job represents a unit of work sent to the worker pool
type Job struct {
	ID       int       `json:"id"`
	Name     string    `json:"name"`
	Duration int       `json:"duration_ms"` // how long the job "works" in ms
	SubmitAt time.Time `json:"submitted_at"`
}

// JobResult is what a worker produces after finishing a job
type JobResult struct {
	JobID      int           `json:"job_id"`
	JobName    string        `json:"job_name"`
	WorkerID   int           `json:"worker_id"`
	Status     string        `json:"status"` // "done" or "failed"
	Duration   time.Duration `json:"duration_ns"`
	FinishedAt time.Time     `json:"finished_at"`
}

// Message is a simple chat-like message (demonstrates channels + fan-in)
type Message struct {
	ID     int       `json:"id"`
	From   string    `json:"from"`
	Text   string    `json:"text"`
	SentAt time.Time `json:"sent_at"`
}

// Stats holds live server metrics (demonstrates atomic + RWMutex)
type Stats struct {
	TotalRequests  int64         `json:"total_requests"`
	ActiveWorkers  int           `json:"active_workers"`
	JobsQueued     int           `json:"jobs_queued"`
	JobsCompleted  int64         `json:"jobs_completed"`
	MessagesStored int           `json:"messages_stored"`
	Uptime         time.Duration `json:"uptime_ns"`
	UptimeHuman    string        `json:"uptime"`
	GOMAXPROCS     int           `json:"gomaxprocs"`
	NumGoroutines  int           `json:"num_goroutines"`
}

// APIResponse wraps every response with metadata
type APIResponse struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ---------------------------------------------------------------------------
// EU visa-sponsoring jobs types  (used by JobStore + /api/listings/* routes)
// ---------------------------------------------------------------------------

// CompanyEntry mirrors one element of job.json at the top level.
type CompanyEntry struct {
	Company       string     `json:"company"`
	CountryFlag   string     `json:"country_flag"`
	Country       string     `json:"country"`
	CareerPageURL string     `json:"career_page_url,omitempty"`
	JobSource     *JobSource `json:"job_source,omitempty"`
	Jobs          []JobItem  `json:"jobs"`
}

// JobItem is a single posting nested inside CompanyEntry.
type JobItem struct {
	Company     string `json:"company"`
	JobTitle    string `json:"job_title"`
	Location    string `json:"location"`
	Country     string `json:"country"`
	CountryFlag string `json:"country_flag"`
	ApplyURL    string `json:"apply_url"`
	Added       string `json:"added"`
}

// JobSource describes how the backend can poll a company's careers feed
// and merge any newly discovered jobs into job.json.
type JobSource struct {
	Type               string `json:"type"`
	URL                string `json:"url"`
	TitleField         string `json:"title_field,omitempty"`
	LocationField      string `json:"location_field,omitempty"`
	ApplyURLField      string `json:"apply_url_field,omitempty"`
	CountryField       string `json:"country_field,omitempty"`
	CountryFlagField   string `json:"country_flag_field,omitempty"`
	JobsPath           string `json:"jobs_path,omitempty"`
	DefaultCountry     string `json:"default_country,omitempty"`
	DefaultCountryFlag string `json:"default_country_flag,omitempty"`
}

// JobView is the flattened, ID-tagged representation served by the API.
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

// JobStats is a lightweight summary returned by GET /api/listings/stats.
type JobStats struct {
	Companies int `json:"companies"`
	Jobs      int `json:"jobs"`
	Countries int `json:"countries"`
}

// ListingsResponse is the envelope returned by GET /api/listings.
type ListingsResponse struct {
	Companies      []CompanyEntry `json:"companies"`
	Jobs           []JobView      `json:"jobs"`
	Countries      []string       `json:"countries"`
	Stats          JobStats       `json:"stats"`
	LastUpdatedISO string         `json:"last_updated_iso"`
}
