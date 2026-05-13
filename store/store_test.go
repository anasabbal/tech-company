package store

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test/model"
)

func TestRefreshSourcesAddsOnlyNewJobs(t *testing.T) {
	today := "May 13, 2026"

	path := filepath.Join(t.TempDir(), "job.json")
	initial := []model.CompanyEntry{
		{
			Company:       "Trustpilot",
			Country:       "Denmark",
			CountryFlag:   "🇩🇰",
			CareerPageURL: "https://business.trustpilot.com/jobs",
			JobSource: &model.JobSource{
				Type:               "greenhouse_board",
				URL:                "https://boards-api.greenhouse.io/v1/boards/trustpilot/jobs",
				DefaultCountry:     "Denmark",
				DefaultCountryFlag: "🇩🇰",
			},
			Jobs: []model.JobItem{
				{
					Company:     "Trustpilot",
					JobTitle:    "Senior Software Engineer",
					Location:    "Copenhagen, Denmark",
					Country:     "Denmark",
					CountryFlag: "🇩🇰",
					ApplyURL:    "https://example.com/jobs/1",
					Added:       today,
				},
			},
		},
	}

	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatalf("marshal initial JSON: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write initial JSON: %v", err)
	}

	store, err := NewJobStore(path)
	if err != nil {
		t.Fatalf("NewJobStore: %v", err)
	}
	store.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := json.Marshal(map[string]interface{}{
				"jobs": []map[string]interface{}{
					{
						"title":        "Senior Software Engineer",
						"absolute_url": "https://example.com/jobs/1",
						"location": map[string]string{
							"name": "Copenhagen, Denmark",
						},
					},
					{
						"title":        "Platform Engineer",
						"absolute_url": "https://example.com/jobs/2",
						"location": map[string]string{
							"name": "Aarhus, Denmark",
						},
					},
					{
						"title":        "Marketing Manager",
						"absolute_url": "https://example.com/jobs/3",
						"location": map[string]string{
							"name": "Copenhagen, Denmark",
						},
					},
				},
			})
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}),
	}

	added, err := store.RefreshSources(context.Background())
	if err != nil {
		t.Fatalf("RefreshSources: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 new job, got %d", added)
	}

	companies, err := readCompaniesFile(path)
	if err != nil {
		t.Fatalf("readCompaniesFile: %v", err)
	}
	if len(companies) != 1 || len(companies[0].Jobs) != 2 {
		t.Fatalf("expected persisted file to contain 2 jobs, got %+v", companies)
	}
	for _, job := range companies[0].Jobs {
		if job.ApplyURL == "https://example.com/jobs/3" {
			t.Fatalf("non-engineering role should not have been added: %+v", job)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
