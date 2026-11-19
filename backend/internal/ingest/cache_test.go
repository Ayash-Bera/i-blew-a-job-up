package ingest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"firstrole/internal/db"
	"firstrole/internal/models"
	"firstrole/internal/tagging"
)

type retentionStore struct {
	*db.Memory
	saves, pruneCalls, failSave int
	pruneErr                    error
	protected                   []string
	cutoff                      time.Time
	publishedAtPrune            bool
}

func (s *retentionStore) Save(ctx context.Context, state models.State, owner string) error {
	s.saves++
	if s.saves == s.failSave {
		return errors.New("save rejected")
	}
	return s.Memory.Save(ctx, state, owner)
}

func (s *retentionStore) PruneTags(ctx context.Context, owner string, protected []string, cutoff time.Time) (int, error) {
	s.pruneCalls++
	s.protected, s.cutoff = protected, cutoff
	s.publishedAtPrune = len(s.State.Jobs) == 1 && s.State.Jobs[0].TaggingMethod == "llm" && s.saves == 2
	if s.pruneErr != nil {
		return 0, s.pruneErr
	}
	return s.Memory.PruneTags(ctx, owner, protected, cutoff)
}

func TestRefreshCachePruning(t *testing.T) {
	for _, tc := range []struct {
		name                                                         string
		partial, pending, missingConfig, failSave, cancel, failPrune bool
		status                                                       string
		calls                                                        int
	}{
		{name: "success", status: "success", calls: 1},
		{name: "partial source", partial: true, status: "partial"},
		{name: "pending tags", pending: true, status: "awaiting_tags"},
		{name: "missing configuration", missingConfig: true, status: "success"},
		{name: "failed commit", failSave: true, status: "failed"},
		{name: "cancelled refresh", cancel: true, status: "timed_out"},
		{name: "maintenance failure", failPrune: true, status: "partial", calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &retentionStore{Memory: db.NewMemory()}
			if tc.failSave {
				store.failSave = 2
			}
			if tc.failPrune {
				store.pruneErr = errors.New("maintenance unavailable")
			}
			client := &tagging.Client{URL: "https://model.invalid", Model: "test", Key: "test"}
			if tc.missingConfig {
				client.Key = ""
			}
			p := raw("retention")
			key := client.CacheKey(p)
			if !tc.pending {
				if err := store.PutTag(context.Background(), models.TagRecord{Key: key, Tags: models.Tags{Role: "Software Engineering", Cities: []string{"Bengaluru"}, LocationStatus: "city", EmploymentType: "internship", WorkMode: "unspecified"}}); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			engine := Engine{Store: store, Tagger: client, TagCacheRetention: 90 * 24 * time.Hour, Fetch: func(context.Context, string) ([]models.SourceResult, error) {
				r := result(p)
				if tc.partial {
					r.Complete = false
					r.Error = "source incomplete"
				}
				if tc.cancel {
					cancel()
				}
				return []models.SourceResult{r}, nil
			}}
			before := time.Now().Add(-engine.TagCacheRetention)
			run, err := engine.Run(ctx)
			if (err != nil) != (tc.failSave || tc.cancel) || run.Status != tc.status || store.pruneCalls != tc.calls {
				t.Fatal(run, err, store.pruneCalls)
			}
			if tc.calls > 0 {
				if !store.publishedAtPrune || !reflect.DeepEqual(store.protected, []string{key}) || store.cutoff.Before(before) || store.cutoff.After(time.Now().Add(-engine.TagCacheRetention)) {
					t.Fatal("pruning ran before final publication or with incorrect protection", store)
				}
				if tc.failPrune {
					if run.CachePruneError != "cache_pruning_failed" || run.CachePruned != nil || run.Errors != 1 {
						t.Fatal(run)
					}
				} else if run.CachePruned == nil || *run.CachePruned != 0 || run.CachePruneError != "" {
					t.Fatal(run)
				}
			} else if run.CachePruned != nil {
				t.Fatal("skipped maintenance reported a result", run)
			}
		})
	}
}
