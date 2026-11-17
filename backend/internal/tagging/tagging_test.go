package tagging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"firstrole/internal/models"
)

func source() models.RawPosting {
	return models.RawPosting{Title: "Software Intern", Locations: []string{"Bangalore, India"}, Description: "Freshers can apply. Build software."}
}
func valid() models.Tags {
	return models.Tags{Role: "Software Engineering", Cities: []string{"Bengaluru"}, LocationStatus: "city", WorkMode: "unspecified", EmploymentType: "internship", RoleEvidence: "Software Intern", LocationEvidence: "Bangalore, India"}
}
func TestStrictValidation(t *testing.T) {
	p := source()
	tags := valid()
	if err := Validate(tags, p); err != nil {
		t.Fatal(err)
	}
	tags.Cities = []string{"Mumbai"}
	if Validate(tags, p) == nil {
		t.Fatal("invented city accepted")
	}
	tags = valid()
	tags.RoleEvidence = "unseen phrase"
	if Validate(tags, p) == nil {
		t.Fatal("invented evidence accepted")
	}
	var decoded models.Tags
	if Decode(`{"role":"Software Engineering"}`, &decoded) == nil {
		t.Fatal("missing fields accepted")
	}
	b, _ := json.Marshal(valid())
	extra := string(b[:len(b)-1]) + `,"extra":true}`
	if Decode(extra, &decoded) == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestUsageAndRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authentication")
		}
		if calls == 1 {
			w.WriteHeader(429)
			return
		}
		tags, _ := json.Marshal(valid())
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(tags)}}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50}})
	}))
	defer server.Close()
	in, out := 0.1, 0.2
	c := Client{URL: server.URL, Model: "test-model", Key: "test-key", InputRate: &in, OutputRate: &out, HTTP: server.Client()}
	rows := []models.Usage{}
	tags, err := c.Tag(context.Background(), source(), "run", "job", "key", func(_ context.Context, u models.Usage) error {
		for i := range rows {
			if rows[i].ID == u.ID {
				rows[i] = u
				return nil
			}
		}
		rows = append(rows, u)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tags.Role != "Software Engineering" || calls != 2 || len(rows) != 2 {
		t.Fatal(tags, calls, rows)
	}
	if rows[0].CostUSD != nil {
		t.Fatal("unknown retry cost invented")
	}
	if rows[1].CostUSD == nil || *rows[1].CostUSD != 0.00002 {
		t.Fatal(rows[1])
	}
}
func TestCacheKeyVersioned(t *testing.T) {
	p := source()
	a := CacheKey(p, "model-a")
	if a == CacheKey(p, "model-b") {
		t.Fatal("model missing from key")
	}
	p.Description += " Extra."
	if a == CacheKey(p, "model-a") {
		t.Fatal("content missing from key")
	}
}

func TestAttemptMustPersistBeforeRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	client := Client{URL: server.URL, Model: "test", Key: "test"}
	_, err := client.Tag(context.Background(), source(), "run", "job", "key", func(context.Context, models.Usage) error { return context.DeadlineExceeded })
	if err == nil || calls != 0 {
		t.Fatal("provider called before durable attempt", calls, err)
	}
}

func TestDuplicateCityAliasesRejected(t *testing.T) {
	tags := valid()
	tags.Cities = []string{"Bangalore", "Bengaluru"}
	if Validate(tags, source()) == nil {
		t.Fatal("duplicate aliases accepted")
	}
}
