package model

import "time"

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
