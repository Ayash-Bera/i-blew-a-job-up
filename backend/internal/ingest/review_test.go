package ingest

import (
	"firstrole/internal/db"
	"firstrole/internal/models"
	"firstrole/internal/tagging"
	"testing"
	"time"
)

func TestGenericApplyURLDoesNotMergeConflictingJobs(t *testing.T) {
	s := db.Empty()
	a, b := raw("a"), raw("b")
	a.RequisitionID = "A"
	b.RequisitionID = "B"
	a.ApplyURL = "https://jobs.example.com/apply"
	b.ApplyURL = a.ApplyURL
	Reconcile(&s, []models.SourceResult{result(a, b)}, nil)
	if len(Candidates(s)) != 2 {
		t.Fatal("conflicting requisitions merged through apply URL")
	}
}

func TestAuthoritativeClosureSuppressesMirror(t *testing.T) {
	s := db.Empty()
	official := raw("official")
	mirror := official
	mirror.SourceID = "mirror"
	mirror.SourceKind = "apna"
	mirror.PostingID = "mirror"
	mirror.URL = "https://board.example.com/mirror"
	mirror.ApplyURL = official.URL
	Reconcile(&s, []models.SourceResult{result(official), {SourceID: "mirror", Complete: true, Posts: []models.RawPosting{mirror}, CheckedAt: time.Now()}}, nil)
	if len(Candidates(s)) != 1 {
		t.Fatal("mirror did not deduplicate")
	}
	Reconcile(&s, []models.SourceResult{result()}, nil)
	if len(Candidates(s)) != 0 {
		t.Fatal("withdrawn employer job survived through mirror")
	}
}

func TestInitialCohortCannotDrift(t *testing.T) {
	p := raw("one")
	client := &tagging.Client{Model: "test"}
	key := client.CacheKey(p)
	now := time.Now()
	state := db.Empty()
	state.InitialCohort = &models.InitialCohort{StartedAt: now, Versions: map[string]string{"one": key}}
	cost := 0.1
	usage := []models.Usage{{JobID: "one", ContentKey: key, CreatedAt: now, CostUSD: &cost}, {JobID: "unrelated", ContentKey: "other", CreatedAt: now, CostUSD: &cost}}
	c := candidate{Raw: p, Job: models.Job{ID: "one"}}
	jobs := []models.Job{{ID: "one", TaggingMethod: "llm"}}
	finishInitialCohort(&state, []candidate{c}, jobs, usage, client)
	if state.Costs.InitialPer1000USD == nil || *state.Costs.InitialPer1000USD != 100 {
		t.Fatal(state.Costs)
	}
	usage = append(usage, models.Usage{JobID: "one", ContentKey: key, CreatedAt: now, CostUSD: &cost})
	finishInitialCohort(&state, []candidate{c}, jobs, usage, client)
	if *state.Costs.InitialPer1000USD != 100 {
		t.Fatal("completed cohort cost changed")
	}
}

func TestSnapshotPreservesPartialSource(t *testing.T) {
	p := raw("partial")
	partial := result(p)
	partial.Complete = false
	state := Snapshot([]models.SourceResult{partial}, nil)
	if len(state.Sources) != 1 || state.Sources[0].Status != "partial" || state.Sources[0].LastSuccessAt != nil {
		t.Fatal(state.Sources)
	}
}

func TestConfirmedClosureSurvivesPartialFetch(t *testing.T) {
	s := db.Empty()
	a, b := raw("a"), raw("b")
	a.RequisitionID = "A"
	b.RequisitionID = "B"
	Reconcile(&s, []models.SourceResult{result(a, b)}, nil)
	partial := result()
	partial.Complete = false
	partial.Error = "one page unavailable"
	partial.ClosedPostingIDs = []string{"a"}
	Reconcile(&s, []models.SourceResult{partial}, nil)
	cs := Candidates(s)
	if len(cs) != 1 || cs[0].Raw.PostingID != "b" {
		t.Fatal("known closure was retained or unrelated job removed", cs)
	}
}

func TestAuthoritativeExpirySuppressesMirror(t *testing.T) {
	s := db.Empty()
	official := raw("official")
	official.ValidThrough = time.Now().Add(time.Hour).Format(time.RFC3339)
	mirror := official
	mirror.SourceID = "mirror"
	mirror.SourceKind = "apna"
	mirror.PostingID = "mirror"
	mirror.URL = "https://board.example.com/mirror"
	mirror.ValidThrough = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	Reconcile(&s, []models.SourceResult{result(official), {SourceID: "mirror", Complete: true, Posts: []models.RawPosting{mirror}, CheckedAt: time.Now()}}, nil)
	cs := Candidates(s)
	if len(cs) != 1 || cs[0].Job.Source != "lever" || cs[0].Job.ValidThrough == nil || cs[0].Job.ValidThrough.Format(time.RFC3339) != official.ValidThrough {
		t.Fatal("authoritative primary/expiry lost", cs)
	}
	o := s.Observations["board:official"]
	o.Raw.ValidThrough = time.Now().Add(-time.Hour).Format(time.RFC3339)
	s.Observations["board:official"] = o
	if len(Candidates(s)) != 0 {
		t.Fatal("expired employer job survived via mirror")
	}
}

func TestConflictingUnderlyingIDsHaveDistinctPublicIDs(t *testing.T) {
	s := db.Empty()
	a, b := raw("a"), raw("b")
	a.SourceKind = "greenhouse"
	b.SourceKind = "greenhouse"
	a.UnderlyingID = "one"
	b.UnderlyingID = "two"
	a.RequisitionID = "shared"
	b.RequisitionID = "shared"
	Reconcile(&s, []models.SourceResult{result(a, b)}, nil)
	cs := Candidates(s)
	if len(cs) != 2 || cs[0].Job.ID == cs[1].Job.ID {
		t.Fatal("conflicting groups share a public identity", cs)
	}
	again := Candidates(s)
	if again[0].Job.ID != cs[0].Job.ID || again[1].Job.ID != cs[1].Job.ID {
		t.Fatal("conflict IDs are unstable")
	}
}

func TestDisabledSourceStatus(t *testing.T) {
	state := db.Empty()
	Reconcile(&state, nil, []models.Source{{ID: "paused", Company: "Employer", Enabled: false}})
	if len(state.Sources) != 1 || state.Sources[0].Status != "disabled" {
		t.Fatal(state.Sources)
	}
}
