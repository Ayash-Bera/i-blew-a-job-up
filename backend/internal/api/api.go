package api

import (
	"context"
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"firstrole/internal/db"
	"firstrole/internal/ingest"
	"firstrole/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	Store              db.Store
	Engine             *ingest.Engine
	Origin, CronSecret string
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			next.ServeHTTP(w, r)
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		})
	})
	r.Use(s.cors)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(20 * time.Second))
		r.Get("/api/jobs", s.jobs)
		r.Get("/api/stats", s.stats)
		r.Get("/api/sources", s.sources)
		r.Get("/api/runs", s.runs)
		r.Get("/api/export.csv", s.export)
	})
	r.With(s.auth).Get("/api/cron", s.refresh)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, 404, map[string]string{"error": "route not found"})
	})
	return r
}
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if s.Origin != "" && r.Header.Get("Origin") == s.Origin {
			w.Header().Set("Access-Control-Allow-Origin", s.Origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("Authorization")
		expected := "Bearer " + s.CronSecret
		if len(s.CronSecret) < 32 || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			JSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "error", err)
	}
}
func (s *Server) load(w http.ResponseWriter, r *http.Request) (models.State, bool) {
	state, err := s.Store.Load(r.Context(), false)
	if err != nil {
		slog.Error("read public state", "error", err)
		JSON(w, 503, map[string]string{"error": "dataset temporarily unavailable"})
		return state, false
	}
	return PublicState(state, time.Now()), true
}

func PublicState(state models.State, now time.Time) models.State {
	active := make([]models.Job, 0, len(state.Jobs))
	counts := map[string]int{}
	state.PendingReview = 0
	for _, job := range state.Jobs {
		if job.ValidThrough != nil && !job.ValidThrough.After(now) || job.LastCheckedAt.Before(now.Add(-48*time.Hour)) {
			continue
		}
		if job.TaggingMethod != "llm" {
			state.PendingReview++
			if state.Mode != "snapshot_read_only" {
				continue
			}
		}
		active = append(active, job)
		for _, id := range job.SourceIDs {
			counts[id]++
		}
	}
	state.Jobs = active
	for i := range state.Sources {
		state.Sources[i].JobCount = counts[state.Sources[i].ID]
	}
	return state
}
func setSorted(set map[string]bool) []string {
	out := []string{}
	for v := range set {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

type Facets struct {
	Roles  []string `json:"roles"`
	Cities []string `json:"cities"`
	Types  []string `json:"types"`
	Modes  []string `json:"modes"`
}
type JobPage struct {
	Jobs   []models.Job `json:"jobs"`
	Total  int          `json:"total"`
	Page   int          `json:"page"`
	Limit  int          `json:"limit"`
	Facets Facets       `json:"facets"`
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	state, ok := s.load(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, limit := 1, 20
	var err error
	if value := q.Get("page"); value != "" {
		page, err = strconv.Atoi(value)
	}
	if err != nil || page < 1 || page > 100000 {
		JSON(w, 400, map[string]string{"error": "page must be between 1 and 100000"})
		return
	}
	if value := q.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
	}
	if err != nil || limit < 1 || limit > 100 {
		JSON(w, 400, map[string]string{"error": "limit must be between 1 and 100"})
		return
	}
	if len(q.Get("q")) > 200 {
		JSON(w, 400, map[string]string{"error": "query exceeds 200 characters"})
		return
	}
	roles, cities, types, modes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	found := []models.Job{}
	for _, j := range state.Jobs {
		roles[j.Role] = true
		types[j.EmploymentType] = true
		modes[j.WorkMode] = true
		cityMatch := q.Get("city") == ""
		for _, city := range j.Cities {
			cities[city] = true
			if city == q.Get("city") {
				cityMatch = true
			}
		}
		if !cityMatch || q.Get("role") != "" && j.Role != q.Get("role") || q.Get("type") != "" && j.EmploymentType != q.Get("type") || q.Get("mode") != "" && j.WorkMode != q.Get("mode") {
			continue
		}
		needle := strings.ToLower(q.Get("q"))
		if needle != "" && !strings.Contains(strings.ToLower(j.Title+" "+j.Company+" "+j.DescriptionExcerpt+" "+strings.Join(j.Cities, " ")), needle) {
			continue
		}
		found = append(found, j)
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].FirstSeenAt.Equal(found[j].FirstSeenAt) {
			return found[i].ID < found[j].ID
		}
		return found[i].FirstSeenAt.After(found[j].FirstSeenAt)
	})
	start := min((page-1)*limit, len(found))
	end := min(start+limit, len(found))
	JSON(w, 200, JobPage{Jobs: found[start:end], Total: len(found), Page: page, Limit: limit, Facets: Facets{Roles: setSorted(roles), Cities: setSorted(cities), Types: setSorted(types), Modes: setSorted(modes)}})
}
func Stats(s models.State) models.Stats {
	stats := models.Stats{ActiveJobs: len(s.Jobs), RefreshStatus: "not_started", Evaluation: s.Evaluation, Costs: s.Costs}
	companies, cities, sources := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, j := range s.Jobs {
		companies[j.Company] = true
		sources[j.Source] = true
		for _, c := range j.Cities {
			cities[c] = true
		}
		if j.TaggingMethod == "llm" {
			stats.TaggedJobs++
		} else {
			stats.PendingReview++
		}
	}
	stats.Companies = len(companies)
	stats.PendingReview = max(stats.PendingReview, s.PendingReview)
	stats.Cities = len(cities)
	stats.Sources = len(sources)
	for _, run := range s.Runs {
		if stats.LastAttemptAt == nil || run.StartedAt.After(*stats.LastAttemptAt) {
			t := run.StartedAt
			stats.LastAttemptAt = &t
			stats.RefreshStatus = run.Status
			if run.Status == "running" && time.Since(t) > 15*time.Minute {
				stats.RefreshStatus = "interrupted"
			}
		}
		if run.Status == "success" && run.FinishedAt != nil && (stats.LastSuccessAt == nil || run.FinishedAt.After(*stats.LastSuccessAt)) {
			stats.LastSuccessAt = run.FinishedAt
		}
	}
	if s.Mode == "snapshot_read_only" {
		stats.RefreshStatus = "snapshot_read_only"
	}
	return stats
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	state, ok := s.load(w, r)
	if ok {
		JSON(w, 200, Stats(state))
	}
}
func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	state, ok := s.load(w, r)
	if ok {
		JSON(w, 200, map[string]any{"sources": state.Sources})
	}
}
func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	state, ok := s.load(w, r)
	if ok {
		JSON(w, 200, map[string]any{"runs": state.Runs})
	}
}
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if s.Engine == nil {
		JSON(w, 503, map[string]string{"error": "refresh requires durable storage and source configuration"})
		return
	}
	run, err := s.Engine.Run(r.Context())
	if err != nil {
		status := 500
		message := "refresh unsuccessful; inspect run status"
		if errors.Is(err, db.ErrReadOnly) {
			status = 503
			message = err.Error()
		}
		if errors.Is(err, ingest.ErrBusy) {
			status = 409
			message = err.Error()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			status = 504
		}
		slog.Error("refresh", "run_id", run.ID, "error", err)
		JSON(w, status, map[string]any{"error": message, "run": run})
		return
	}
	JSON(w, 200, map[string]any{"run": run})
}

func SafeCSV(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r\n", rune(s[0])) {
		return "'" + s
	}
	return s
}
func WriteCSV(w *csv.Writer, jobs []models.Job) error {
	if err := w.Write([]string{"id", "title", "company", "role", "cities", "location_status", "work_mode", "employment_type", "source", "source_url", "apply_url", "eligibility_evidence", "first_seen_at", "last_checked_at", "published_at", "tagging_method", "description_excerpt"}); err != nil {
		return err
	}
	for _, j := range jobs {
		published := ""
		if j.PublishedAt != nil {
			published = j.PublishedAt.Format(time.RFC3339)
		}
		row := []string{j.ID, j.Title, j.Company, j.Role, strings.Join(j.Cities, " | "), j.LocationStatus, j.WorkMode, j.EmploymentType, j.Source, j.SourceURL, j.ApplyURL, j.EligibilityEvidence, j.FirstSeenAt.Format(time.RFC3339), j.LastCheckedAt.Format(time.RFC3339), published, j.TaggingMethod, j.DescriptionExcerpt}
		for i := range row {
			row[i] = SafeCSV(row[i])
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	state, ok := s.load(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=i-blew-a-job-up-jobs.csv")
	if err := WriteCSV(csv.NewWriter(w), state.Jobs); err != nil {
		slog.Error("export dataset", "error", err)
	}
}
