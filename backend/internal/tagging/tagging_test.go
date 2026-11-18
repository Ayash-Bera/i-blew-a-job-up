package tagging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	if rows[0].HTTPStatus != http.StatusTooManyRequests || rows[1].HTTPStatus != http.StatusOK {
		t.Fatal("incorrect response status", rows[0].HTTPStatus, rows[1].HTTPStatus)
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

func TestCacheIdentityIncludesProviderAndSettings(t *testing.T) {
	p := source()
	client := &Client{Model: "same-model", URL: "https://example.com/v1/chat/completions", Key: "first"}
	original := client.CacheKey(p)
	client.Key = "rotated"
	client.Provider = "compatible"
	client.MinInterval = time.Second
	rate := 0.5
	client.InputRate = &rate
	if client.CacheKey(p) != original {
		t.Fatal("credential rotation, pricing, or pacing invalidated tags")
	}
	client.Provider = "gemini"
	if client.CacheKey(p) == original {
		t.Fatal("provider and generation settings missing from cache identity")
	}
	client.Provider = "compatible"
	client.URL = "https://other.example.com/v1/chat/completions"
	if client.CacheKey(p) == original {
		t.Fatal("endpoint missing from cache identity")
	}
	client.URL = "https://example.com/v1/chat/completions"
	client.Model = "new-model"
	if client.CacheKey(p) == original {
		t.Fatal("model missing from cache identity")
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

func TestRegionOnlyLocations(t *testing.T) {
	for _, tc := range []struct{ region, city string }{
		{"Goa", "Panaji"},
		{"Tehri Garhwal", "New Tehri"},
		{"  TEHRI  GARHWAL  ", "New Tehri"},
	} {
		t.Run(tc.region, func(t *testing.T) {
			p := source()
			p.Locations = []string{tc.region}
			tags := valid()
			tags.Cities = []string{tc.region}
			tags.LocationEvidence = tc.region
			if Validate(tags, p) == nil {
				t.Fatal("region accepted as a city")
			}
			tags.Cities = []string{}
			tags.LocationStatus = "region"
			if err := Validate(tags, p); err != nil {
				t.Fatal("supported region rejected", err)
			}
			p.Locations = []string{tc.city + ", " + tc.region}
			tags.Cities = []string{tc.city}
			tags.LocationStatus = "city"
			tags.LocationEvidence = p.Locations[0]
			if err := Validate(tags, p); err != nil {
				t.Fatal("explicit settlement rejected", err)
			}
		})
	}
}

func TestInternshipEmploymentType(t *testing.T) {
	for _, kind := range []string{"Intern: Full Time", "Intern: Part Time", "Internship", "INTERN"} {
		t.Run(kind, func(t *testing.T) {
			p := source()
			p.EmploymentType = kind
			tags := valid()
			tags.EmploymentType = "full-time"
			if Validate(tags, p) == nil {
				t.Fatal("internship reduced to working hours")
			}
			tags.EmploymentType = "internship"
			if err := Validate(tags, p); err != nil {
				t.Fatal("internship rejected", err)
			}
		})
	}
	p := source()
	p.EmploymentType = "Internet Services: Full Time"
	tags := valid()
	tags.EmploymentType = "full-time"
	if err := Validate(tags, p); err != nil {
		t.Fatal("unrelated word treated as internship", err)
	}
}

func TestCountryLocationEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, country, evidence string
		want                    bool
	}{
		{"source code", "IN", "IN", true},
		{"country name", "IN", "India", true},
		{"three letter code", "IND", "India", true},
		{"source name", "India", "India", true},
		{"invented country", "IN", "Nepal", false},
		{"absent country", "", "India", false},
		{"country substring", "IN", "Ind", false},
		{"extra city", "IN", "India; Mumbai", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := source()
			p.Title, p.Description = "Hostess", "No experience required."
			p.Locations = nil
			p.Country = tc.country
			tags := valid()
			tags.Role, tags.RoleEvidence = "Business and Operations", p.Title
			tags.Cities = []string{}
			tags.LocationStatus = "region"
			tags.LocationEvidence = tc.evidence
			if err := Validate(tags, p); (err == nil) != tc.want {
				t.Fatalf("Validate() = %v, want accepted %v", err, tc.want)
			}
		})
	}
	p := source()
	p.Title, p.Description = "Hostess", "No experience required."
	p.Locations, p.Country = nil, "IN"
	tags := valid()
	tags.Cities, tags.LocationStatus, tags.LocationEvidence = []string{}, "region", "India"
	for _, evidence := range []string{"India", "IN"} {
		tags.RoleEvidence = evidence
		if err := Validate(tags, p); err == nil || err.Error() != "role evidence absent from source" {
			t.Fatal("country accepted as role evidence", evidence, err)
		}
	}
	tags.RoleEvidence = p.Title
	tags.Cities, tags.LocationStatus = []string{"India"}, "city"
	if Validate(tags, p) == nil {
		t.Fatal("country accepted as a city")
	}
	tags.Cities = []string{"Mumbai"}
	if Validate(tags, p) == nil {
		t.Fatal("country evidence licensed an unsupported city")
	}
}

func TestJoinedLocationEvidence(t *testing.T) {
	p := source()
	p.Locations = []string{"Gurugram, Haryana", "Noida, Uttar Pradesh"}
	p.Description += " Benefits include trips to Mumbai."
	for _, tc := range []struct {
		name, evidence string
		cities         []string
		want           bool
	}{
		{"semicolon", "Gurugram; Noida", []string{"Gurugram", "Noida"}, true},
		{"comma", "Gurugram, Noida", []string{"Gurugram", "Noida"}, true},
		{"pipe", "Gurugram | Noida", []string{"Gurugram", "Noida"}, true},
		{"full locations", "Gurugram, Haryana; Noida, Uttar Pradesh", []string{"Gurugram", "Noida"}, true},
		{"mixed delimiters", "Gurugram, Haryana | Noida; Uttar Pradesh", []string{"Gurugram", "Noida"}, true},
		{"verbatim location", "Gurugram, Haryana", []string{"Gurugram"}, true},
		{"invented fragment", "Gurugram; Noida; Pune", []string{"Gurugram", "Noida"}, false},
		{"description fragment", "Gurugram; Noida; Mumbai", []string{"Gurugram", "Noida", "Mumbai"}, false},
		{"partial fragment", "Gurugram; Noi", []string{"Gurugram", "Noida"}, false},
		{"empty fragment", "Gurugram;; Noida", []string{"Gurugram", "Noida"}, false},
		{"unsupported city", "Gurugram; Noida", []string{"Gurugram", "Noida", "Pune"}, false},
		{"partial city token", "Gurugram; Noida", []string{"Gurugram", "Noi"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tags := valid()
			tags.Cities, tags.LocationEvidence = tc.cities, tc.evidence
			if err := Validate(tags, p); (err == nil) != tc.want {
				t.Fatalf("Validate() = %v, want accepted %v", err, tc.want)
			}
		})
	}
}

func TestObservedCitySpelling(t *testing.T) {
	p := source()
	p.Locations = []string{"Swai Madhopur"}
	tags := valid()
	tags.Cities, tags.LocationEvidence = []string{"Sawai Madhopur"}, "Swai Madhopur"
	if err := Validate(tags, p); err != nil {
		t.Fatal("canonical city rejected for observed source spelling", err)
	}
	tags.Cities = []string{"Jaipur"}
	if Validate(tags, p) == nil {
		t.Fatal("unrelated city accepted for observed source spelling")
	}
	tags.Cities, tags.LocationEvidence = []string{"Sawai Madhopur"}, "Sawai Madhopur"
	if Validate(tags, p) == nil {
		t.Fatal("rewritten evidence accepted instead of verbatim source")
	}
}
