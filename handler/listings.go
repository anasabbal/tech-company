package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"test/model"
	"test/store"
)

// ListingsHandler wires the JobStore to HTTP routes.
// Register with:
//
//	h := handler.NewListingsHandler(jobStore)
//	mux.HandleFunc("/api/listings",          h.GetAll)
//	mux.HandleFunc("/api/listings/filter",   h.Filter)
//	mux.HandleFunc("/api/listings/stats",    h.GetStats)
//	mux.HandleFunc("/api/listings/countries",h.GetCountries)
type ListingsHandler struct {
	store *store.JobStore
}

func NewListingsHandler(s *store.JobStore) *ListingsHandler {
	return &ListingsHandler{store: s}
}

// GET /api/listings
// Returns the full ListingsResponse (companies + flat jobs + countries + stats).
func (h *ListingsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := model.ListingsResponse{
		Companies:      h.store.GetAllCompanies(),
		Jobs:           h.store.GetAllJobs(),
		Countries:      h.store.GetCountries(),
		Stats:          h.store.Stats(),
		LastUpdatedISO: time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, model.APIResponse{OK: true, Data: resp})
}

// GET|POST /api/listings/refresh
// Fetches jobs from configured company sources and persists any new postings.
func (h *ListingsHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	added, err := h.store.RefreshSources(context.Background())
	if err != nil && added == 0 {
		writeJSON(w, http.StatusBadGateway, model.APIResponse{
			OK:      false,
			Message: err.Error(),
		})
		return
	}

	message := "refresh completed"
	if err != nil {
		message = err.Error()
	}

	writeJSON(w, http.StatusOK, model.APIResponse{
		OK:      true,
		Message: message,
		Data: map[string]interface{}{
			"added":           added,
			"stats":           h.store.Stats(),
			"last_refreshed":  h.store.GetLastRefresh().Format(time.RFC3339),
			"total_companies": len(h.store.GetAllCompanies()),
			"total_countries": len(h.store.GetCountries()),
			"total_job_posts": len(h.store.GetAllJobs()),
		},
	})
}

// GET /api/listings/filter?country=Germany&search=backend
// Returns filtered flat job list.
func (h *ListingsHandler) Filter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	country := r.URL.Query().Get("country")
	search := r.URL.Query().Get("search")
	jobs := h.store.Filter(country, search)

	writeJSON(w, http.StatusOK, model.APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"jobs":  jobs,
			"total": len(jobs),
		},
	})
}

// GET /api/listings/stats
func (h *ListingsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{OK: true, Data: h.store.Stats()})
}

// GET /api/listings/countries
func (h *ListingsHandler) GetCountries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, model.APIResponse{OK: true, Data: h.store.GetCountries()})
}

// ---- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, model.APIResponse{OK: false, Message: msg})
}
