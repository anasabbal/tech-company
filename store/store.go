package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"test/model"
)

// JobStore loads job listings from job.json, keeps an in-memory index,
// and can enrich companies with newly discovered jobs from configured sources.
type JobStore struct {
	mu          sync.RWMutex
	path        string
	companies   []model.CompanyEntry
	jobs        []model.JobView
	countries   []string
	httpClient  *http.Client
	lastRefresh time.Time
}

func NewJobStore(path string) (*JobStore, error) {
	s := &JobStore{
		path: path,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads job.json and builds the in-memory indexes.
func (s *JobStore) load() error {
	companies, err := readCompaniesFile(s.path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.companies = companies
	s.rebuildLocked()

	log.Printf("[JobStore] loaded %d companies, %d jobs, %d countries from %s",
		len(s.companies), len(s.jobs), len(s.countries), s.path)
	return nil
}

// RefreshSources fetches jobs from configured company sources, merges new
// postings into the in-memory store, and persists the updated JSON file.
func (s *JobStore) RefreshSources(ctx context.Context) (int, error) {
	s.mu.RLock()
	companies := cloneCompanies(s.companies)
	s.mu.RUnlock()

	type pending struct {
		index int
		jobs  []model.JobItem
	}

	var (
		found []pending
		errs  []string
	)

	for i, company := range companies {
		if company.JobSource == nil {
			continue
		}
		jobs, err := s.fetchSourceJobs(ctx, company)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", company.Company, err))
			continue
		}
		if len(jobs) == 0 {
			continue
		}
		found = append(found, pending{index: i, jobs: jobs})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range found {
		existing := existingApplyURLs(companies[item.index].Jobs)
		for _, job := range item.jobs {
			key := normalizeURL(job.ApplyURL)
			if key == "" || existing[key] {
				continue
			}
			companies[item.index].Jobs = append(companies[item.index].Jobs, job)
			existing[key] = true
		}
	}

	if companiesEqual(companies, s.companies) {
		s.lastRefresh = time.Now().UTC()
		if len(errs) > 0 {
			return 0, errors.New(strings.Join(errs, "; "))
		}
		return 0, nil
	}

	if err := writeCompaniesFile(s.path, companies); err != nil {
		return 0, err
	}

	before := len(s.jobs)
	s.companies = companies
	s.rebuildLocked()
	s.lastRefresh = time.Now().UTC()
	added := len(s.jobs) - before

	if len(errs) > 0 {
		return added, errors.New(strings.Join(errs, "; "))
	}
	return added, nil
}

func (s *JobStore) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			added, err := s.RefreshSources(ctx)
			if err != nil {
				log.Printf("[JobStore] auto refresh error: %v", err)
				continue
			}
			if added > 0 {
				log.Printf("[JobStore] auto refresh added %d new jobs", added)
			}
		}
	}
}

func (s *JobStore) GetLastRefresh() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRefresh
}

// GetAllJobs returns a copy of the flat job list.
func (s *JobStore) GetAllJobs() []model.JobView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.JobView, len(s.jobs))
	copy(out, s.jobs)
	return out
}

// GetAllCompanies returns a copy of the company list.
func (s *JobStore) GetAllCompanies() []model.CompanyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCompanies(s.companies)
}

// GetCountries returns the sorted country list.
func (s *JobStore) GetCountries() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.countries))
	copy(out, s.countries)
	return out
}

// Stats returns a snapshot of job-related counts.
func (s *JobStore) Stats() model.JobStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return model.JobStats{
		Companies: len(s.companies),
		Jobs:      len(s.jobs),
		Countries: len(s.countries),
	}
}

// Filter returns jobs matching the given country and/or search term.
func (s *JobStore) Filter(country, search string) []model.JobView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	search = strings.ToLower(strings.TrimSpace(search))
	country = strings.TrimSpace(country)

	out := make([]model.JobView, 0)
	for _, j := range s.jobs {
		if country != "" && !strings.EqualFold(j.Country, country) {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(j.Company + " " + j.JobTitle + " " + j.Location)
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		out = append(out, j)
	}
	return out
}

func (s *JobStore) rebuildLocked() {
	jobs := make([]model.JobView, 0)
	countrySet := make(map[string]struct{})

	for ci, company := range s.companies {
		if company.Country != "" {
			countrySet[company.Country] = struct{}{}
		}
		for ji, job := range company.Jobs {
			if job.Country != "" {
				countrySet[job.Country] = struct{}{}
			}
			jobs = append(jobs, model.JobView{
				ID:          buildJobID(company.Company, ci, ji),
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
	for c := range countrySet {
		countries = append(countries, c)
	}
	sort.Strings(countries)

	s.jobs = jobs
	s.countries = countries
}

func (s *JobStore) fetchSourceJobs(ctx context.Context, company model.CompanyEntry) ([]model.JobItem, error) {
	switch strings.ToLower(strings.TrimSpace(company.JobSource.Type)) {
	case "greenhouse_board":
		return s.fetchGreenhouseJobs(ctx, company)
	case "json":
		return s.fetchGenericJSONJobs(ctx, company)
	default:
		return nil, fmt.Errorf("unsupported job_source.type %q", company.JobSource.Type)
	}
}

func (s *JobStore) fetchGreenhouseJobs(ctx context.Context, company model.CompanyEntry) ([]model.JobItem, error) {
	var payload struct {
		Jobs []struct {
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}

	if err := s.getJSON(ctx, company.JobSource.URL, &payload); err != nil {
		return nil, err
	}

	items := make([]model.JobItem, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		if strings.TrimSpace(job.Title) == "" || strings.TrimSpace(job.AbsoluteURL) == "" {
			continue
		}
		if !isEngineeringRole(job.Title) {
			continue
		}
		items = append(items, model.JobItem{
			Company:     company.Company,
			JobTitle:    strings.TrimSpace(job.Title),
			Location:    fallbackString(job.Location.Name, company.Country),
			Country:     fallbackString(company.JobSource.DefaultCountry, company.Country),
			CountryFlag: fallbackString(company.JobSource.DefaultCountryFlag, company.CountryFlag),
			ApplyURL:    strings.TrimSpace(job.AbsoluteURL),
			Added:       time.Now().Format("January 2, 2006"),
		})
	}

	return items, nil
}

func (s *JobStore) fetchGenericJSONJobs(ctx context.Context, company model.CompanyEntry) ([]model.JobItem, error) {
	var payload interface{}
	if err := s.getJSON(ctx, company.JobSource.URL, &payload); err != nil {
		return nil, err
	}

	items, err := extractArrayByPath(payload, company.JobSource.JobsPath)
	if err != nil {
		return nil, err
	}

	out := make([]model.JobItem, 0, len(items))
	for _, raw := range items {
		obj, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		title := fieldString(obj, company.JobSource.TitleField)
		applyURL := fieldString(obj, company.JobSource.ApplyURLField)
		if strings.TrimSpace(title) == "" || strings.TrimSpace(applyURL) == "" {
			continue
		}
		if !isEngineeringRole(title) {
			continue
		}

		country := fallbackString(fieldString(obj, company.JobSource.CountryField), company.JobSource.DefaultCountry, company.Country)
		countryFlag := fallbackString(fieldString(obj, company.JobSource.CountryFlagField), company.JobSource.DefaultCountryFlag, company.CountryFlag)

		out = append(out, model.JobItem{
			Company:     company.Company,
			JobTitle:    title,
			Location:    fallbackString(fieldString(obj, company.JobSource.LocationField), company.Country),
			Country:     country,
			CountryFlag: countryFlag,
			ApplyURL:    applyURL,
			Added:       time.Now().Format("January 2, 2006"),
		})
	}

	return out, nil
}

func (s *JobStore) getJSON(ctx context.Context, target string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func readCompaniesFile(path string) ([]model.CompanyEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var companies []model.CompanyEntry
	if err := json.Unmarshal(data, &companies); err != nil {
		return nil, err
	}
	return companies, nil
}

func writeCompaniesFile(path string, companies []model.CompanyEntry) error {
	data, err := json.MarshalIndent(companies, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "job-json-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func cloneCompanies(in []model.CompanyEntry) []model.CompanyEntry {
	out := make([]model.CompanyEntry, len(in))
	for i, company := range in {
		out[i] = company
		out[i].Jobs = append([]model.JobItem(nil), company.Jobs...)
	}
	return out
}

func companiesEqual(a, b []model.CompanyEntry) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func existingApplyURLs(jobs []model.JobItem) map[string]bool {
	out := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		key := normalizeURL(job.ApplyURL)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	return strings.TrimSpace(u.String())
}

func extractArrayByPath(payload interface{}, path string) ([]interface{}, error) {
	if strings.TrimSpace(path) == "" {
		switch typed := payload.(type) {
		case []interface{}:
			return typed, nil
		default:
			return nil, errors.New("jobs_path is required when the response root is not an array")
		}
	}

	current := payload
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %q does not point to an object", path)
		}
		current, ok = obj[part]
		if !ok {
			return nil, fmt.Errorf("path %q not found", path)
		}
	}

	arr, ok := current.([]interface{})
	if !ok {
		return nil, fmt.Errorf("path %q does not point to an array", path)
	}
	return arr, nil
}

func fieldString(obj map[string]interface{}, field string) string {
	if strings.TrimSpace(field) == "" {
		return ""
	}

	current := interface{}(obj)
	for _, part := range strings.Split(field, ".") {
		next, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = next[part]
		if !ok {
			return ""
		}
	}

	switch v := current.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isEngineeringRole(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	if normalized == "" {
		return false
	}

	keywords := []string{
		"software engineer",
		"software developer",
		"backend engineer",
		"backend developer",
		"frontend engineer",
		"frontend developer",
		"full stack engineer",
		"full-stack engineer",
		"full stack developer",
		"full-stack developer",
		"platform engineer",
		"site reliability engineer",
		"sre",
		"devops engineer",
		"machine learning engineer",
		"ml engineer",
		"data engineer",
		"security engineer",
		"infrastructure engineer",
		"systems engineer",
		"embedded engineer",
		"golang engineer",
		"go engineer",
		"java engineer",
		"python engineer",
		"ruby engineer",
		"staff engineer",
		"principal engineer",
		"engineering manager",
		"developer experience engineer",
	}

	for _, keyword := range keywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	// Catch broader titles like "Senior Engineer" while excluding non-tech roles.
	if strings.Contains(normalized, "engineer") || strings.Contains(normalized, "developer") {
		excluded := []string{
			"sales",
			"support",
			"customer",
			"marketing",
			"account",
			"solution",
			"solutions",
			"quality assurance",
			"qa ",
			"recruit",
			"talent",
			"civil",
			"mechanical",
			"electrical",
			"project",
			"product manager",
			"designer",
			"analyst",
		}
		for _, word := range excluded {
			if strings.Contains(normalized, word) {
				return false
			}
		}
		return true
	}

	return false
}

func buildJobID(company string, ci, ji int) string {
	slug := strings.ToLower(strings.ReplaceAll(company, " ", "-"))
	slug = strings.ReplaceAll(slug, "/", "-")
	return slug + "-" + itoa(ci) + "-" + itoa(ji)
}

func parseAdded(value string) time.Time {
	t, err := time.Parse("January 2, 2006", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return t
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
