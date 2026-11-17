package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"firstrole/internal/db"
	"firstrole/internal/models"
	"firstrole/internal/normalize"
	"firstrole/internal/tagging"
)

func raw(id string) models.RawPosting {
	return models.RawPosting{SourceID: "board", SourceKind: "lever", PostingID: id, Title: "Software Intern", Company: "Example", Country: "IN", Locations: []string{"Bangalore, India"}, Description: "Freshers can apply. Build software.", URL: "https://jobs.example.com/" + id, FetchedAt: time.Now().UTC()}
}
func result(posts ...models.RawPosting) models.SourceResult {
	return models.SourceResult{SourceID: "board", Complete: true, Posts: posts, CheckedAt: time.Now().UTC()}
}
func TestReconcileCompleteOnly(t *testing.T) {
	s := db.Empty()
	p := raw("1")
	Reconcile(&s, []models.SourceResult{result(p)}, nil)
	if len(Candidates(s)) != 1 {
		t.Fatal("expected active job")
	}
	partial := result()
	partial.Complete = false
	partial.Error = "source unavailable"
	Reconcile(&s, []models.SourceResult{partial}, nil)
	if len(Candidates(s)) != 1 {
		t.Fatal("partial fetch removed job")
	}
	Reconcile(&s, []models.SourceResult{result()}, nil)
	if len(Candidates(s)) != 0 || s.Observations[normalize.Identity(p)].MissingCount != 1 {
		t.Fatal("first complete absence must hide job")
	}
	Reconcile(&s, []models.SourceResult{result()}, nil)
	if s.Observations[normalize.Identity(p)].MissingCount != 2 {
		t.Fatal("second absence not tracked")
	}
	Reconcile(&s, []models.SourceResult{result(p)}, nil)
	if len(Candidates(s)) != 1 {
		t.Fatal("reappearance lost")
	}
}
func TestDedupAndDistinctRequisitions(t *testing.T) {
	for _, tt := range []struct {
		name       string
		kind       string
		reqA, reqB string
		expected   int
	}{{"exact mirror", "ashby", "", "", 1}, {"distinct requisitions", "ashby", "A", "B", 2}, {"same requisition", "ashby", "A", "A", 1}} {
		t.Run(tt.name, func(t *testing.T) {
			s := db.Empty()
			p, q := raw("1"), raw("2")
			p.RequisitionID = tt.reqA
			q.RequisitionID = tt.reqB
			q.SourceKind = tt.kind
			Reconcile(&s, []models.SourceResult{result(p, q)}, nil)
			if got := len(Candidates(s)); got != tt.expected {
				t.Fatalf("got %d want %d", got, tt.expected)
			}
		})
	}
}
func TestModelCacheNoSecondCall(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		tags := models.Tags{Role: "Software Engineering", Cities: []string{"Bengaluru"}, LocationStatus: "city", WorkMode: "unspecified", EmploymentType: "internship", RoleEvidence: "Software Intern", LocationEvidence: "Bangalore, India"}
		b, _ := json.Marshal(tags)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(b)}}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50}})
	}))
	defer model.Close()
	store := db.NewMemory()
	p := raw("1")
	rate := 0.1
	engine := Engine{Store: store, Fetch: func(context.Context, string) ([]models.SourceResult, error) {
		return []models.SourceResult{result(p)}, nil
	}, Tagger: &tagging.Client{URL: model.URL, Key: "test", Model: "test", InputRate: &rate, OutputRate: &rate}, MaxTags: 10, Concurrency: 2}
	for i := 0; i < 2; i++ {
		run, err := engine.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "success" {
			t.Fatal(run)
		}
	}
	s, _ := store.Load(context.Background(), false)
	if calls != 1 || len(s.Jobs) != 1 || s.Jobs[0].TaggingMethod != "llm" {
		t.Fatal(calls, s.Jobs)
	}
	if s.Costs.TotalUSD == nil {
		t.Fatal("usage not retained")
	}
}
func TestMissingModelHonestyAndLease(t *testing.T) {
	store := db.NewMemory()
	engine := Engine{Store: store, Fetch: func(context.Context, string) ([]models.SourceResult, error) {
		return []models.SourceResult{result(raw("1"))}, nil
	}}
	run, err := engine.Run(context.Background())
	if err != nil || run.Status != "awaiting_tags" {
		t.Fatal(run, err)
	}
	s, _ := store.Load(context.Background(), false)
	if s.Jobs[0].Role != "Unknown" || s.Costs.TotalUSD != nil {
		t.Fatal("invented model output")
	}
	store.Acquire(context.Background(), "other", time.Minute)
	if _, err = engine.Run(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
}
func TestCostsIncludeRetriesOncePerVersion(t *testing.T) {
	one, two := 0.1, 0.2
	usage := []models.Usage{{JobID: "a", ContentKey: "v1", CostUSD: &one}, {JobID: "a", ContentKey: "v1", CostUSD: &two}}
	cost := Costs(usage, nil, nil, "")
	if cost.TaggingPer1000USD == nil || *cost.TaggingPer1000USD < 299.99 || *cost.TaggingPer1000USD > 300.01 {
		t.Fatal(cost)
	}
	usage = append(usage, models.Usage{JobID: "b"})
	if Costs(usage, nil, nil, "").TotalUSD != nil {
		t.Fatal("missing usage treated as zero")
	}
}

func TestTimedOutRunCommitsObservedPosts(t *testing.T) {
	store := db.NewMemory()
	p := raw("timeout")
	engine := Engine{Store: store, Timeout: 50 * time.Millisecond, Fetch: func(ctx context.Context, _ string) ([]models.SourceResult, error) {
		<-ctx.Done()
		partial := result(p)
		partial.Complete = false
		partial.Error = "source fetch exceeded deadline"
		return []models.SourceResult{partial}, ctx.Err()
	}}
	run, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "partial" {
		t.Fatal(run)
	}
	s, _ := store.Load(context.Background(), true)
	if len(s.Observations) != 1 || len(s.Jobs) != 1 {
		t.Fatal("partial observations were lost")
	}
}

func TestSameUnderlyingJobKeepsEveryLocation(t *testing.T) {
	s := db.Empty()
	a, b := raw("a"), raw("b")
	a.SourceKind = "greenhouse"
	b.SourceKind = "greenhouse"
	a.UnderlyingID = "same"
	b.UnderlyingID = "same"
	b.Locations = []string{"Pune, India"}
	Reconcile(&s, []models.SourceResult{result(a, b)}, nil)
	cs := Candidates(s)
	if len(cs) != 1 || len(cs[0].Raw.Locations) != 2 {
		t.Fatal(cs)
	}
}

func TestDistinctUnderlyingJobsNotMergedByText(t *testing.T) {
	s := db.Empty()
	a, b := raw("a"), raw("b")
	a.SourceKind = "greenhouse"
	b.SourceKind = "greenhouse"
	a.UnderlyingID = "one"
	b.UnderlyingID = "two"
	Reconcile(&s, []models.SourceResult{result(a, b)}, nil)
	if len(Candidates(s)) != 2 {
		t.Fatal("distinct employer jobs were merged")
	}
}
