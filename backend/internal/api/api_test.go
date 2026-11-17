package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firstrole/internal/db"
	"firstrole/internal/models"
)

func TestFiltersPaginationAndExpiry(t *testing.T) {
	store := db.NewMemory()
	old := time.Now().Add(-time.Hour)
	store.State.Jobs = []models.Job{{ID: "1", Title: "Developer", Company: "Acme", Role: "Software Engineering", Cities: []string{"Bengaluru"}, EmploymentType: "internship", TaggingMethod: "pending"}, {ID: "2", Title: "Analyst", Company: "Acme", Role: "Data and Analytics", Cities: []string{"Pune"}}, {ID: "3", Title: "Expired", Company: "Acme", Role: "Other", ValidThrough: &old}}
	store.State.Mode = "snapshot_read_only"
	for i := range store.State.Jobs {
		store.State.Jobs[i].LastCheckedAt = time.Now()
	}
	router := (&Server{Store: store}).Router()
	for _, tt := range []struct {
		url           string
		status, total int
	}{{"/api/jobs", 200, 2}, {"/api/jobs?city=Bengaluru&type=internship", 200, 1}, {"/api/jobs?q=analyst", 200, 1}, {"/api/jobs?page=100", 200, 2}, {"/api/jobs?limit=1000", 400, 0}, {"/api/jobs?page=-1", 400, 0}} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", tt.url, nil))
		if rec.Code != tt.status {
			t.Fatal(tt.url, rec.Code)
		}
		if tt.status == 200 {
			var page JobPage
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Total != tt.total {
				t.Fatal(tt.url, page)
			}
		}
	}
}
func TestCronAuthenticationAndNoFalseMetrics(t *testing.T) {
	store := db.NewMemory()
	server := &Server{Store: store, CronSecret: strings.Repeat("x", 32)}
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/api/cron", nil))
	if rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	request := httptest.NewRequest("GET", "/api/cron", nil)
	request.Header.Set("Authorization", "Bearer "+server.CronSecret)
	rec = httptest.NewRecorder()
	server.Router().ServeHTTP(rec, request)
	if rec.Code != 503 {
		t.Fatal(rec.Code)
	}
	state, _ := store.Load(context.Background(), false)
	stats := Stats(state)
	if stats.Evaluation.JointAccuracy != nil || stats.Costs.TotalUSD != nil || stats.Evaluation.Reviewed != 0 {
		t.Fatal(stats)
	}
}
func TestCSVFormulaProtection(t *testing.T) {
	for _, s := range []string{"=SUM(A1)", "+cmd", "-2", "@x", "\tcommand", "\rcommand", "\ncommand"} {
		if SafeCSV(s) != "'"+s {
			t.Fatal(s)
		}
	}
	if SafeCSV("Software Intern") != "Software Intern" {
		t.Fatal("ordinary value changed")
	}
}

func TestPublicationAndFreshness(t *testing.T) {
	now := time.Now()
	state := db.Empty()
	state.Jobs = []models.Job{{ID: "tagged", TaggingMethod: "llm", LastCheckedAt: now}, {ID: "pending", TaggingMethod: "pending", LastCheckedAt: now}, {ID: "stale", TaggingMethod: "llm", LastCheckedAt: now.Add(-49 * time.Hour)}}
	public := PublicState(state, now)
	stats := Stats(public)
	if len(public.Jobs) != 1 || stats.PendingReview != 1 || stats.TaggedJobs != 1 {
		t.Fatal(public, stats)
	}
	state.Mode = "snapshot_read_only"
	snapshot := PublicState(state, now)
	if len(snapshot.Jobs) != 2 || Stats(snapshot).PendingReview != 1 {
		t.Fatal(snapshot)
	}
}
