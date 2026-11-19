package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"firstrole/internal/normalize"
)

func TestWorkdayDetailLocationControlsEligibility(t *testing.T) {
	for _, test := range []struct {
		name, country, location string
		eligible                bool
	}{
		{"India country", "India", "Bengaluru", true},
		{"explicit location", "", "Pune, India", true},
		{"foreign country", "United States of America", "Bengaluru", false},
		{"unspecified location", "", "Remote", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/job/") {
					json.NewEncoder(w).Encode(map[string]any{"jobPostingInfo": map[string]any{
						"jobReqId": "JR1", "title": "Student Intern", "jobDescription": "Currently enrolled university student",
						"location": test.location, "country": map[string]string{"descriptor": test.country},
					}})
					return
				}
				var request struct {
					Facets map[string][]string `json:"appliedFacets"`
					Search string              `json:"searchText"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if len(request.Facets) == 0 {
					if request.Search != "" {
						t.Error("country discovery was narrowed by the job search")
					}
					fmt.Fprint(w, `{"jobPostings":[],"total":1,"facets":[{"facetParameter":"locationCountry","values":[{"descriptor":"India","id":"in"}]}]}`)
					return
				}
				if request.Search != "intern" || len(request.Facets["locationCountry"]) != 1 || request.Facets["locationCountry"][0] != "in" {
					t.Error("job search lost its explicit India filter")
				}
				fmt.Fprint(w, `{"jobPostings":[{"title":"Student Intern","externalPath":"/job/one"}],"total":1}`)
			}))
			defer server.Close()
			source := testSource("workday")
			source.URL, source.Search = server.URL+"/External", "intern"
			posts, err := newCollector().workday(context.Background(), source)
			if err != nil || len(posts) != 1 {
				t.Fatalf("collect: %+v %v", posts, err)
			}
			if posts[0].Country != test.country {
				t.Fatalf("detail country replaced: %q", posts[0].Country)
			}
			got, _, reason := normalize.Eligibility(posts[0])
			if got != test.eligible {
				t.Fatalf("eligible=%v, want %v: %s", got, test.eligible, reason)
			}
		})
	}
}

func TestWorkdayChangedPositiveTotalIsIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Facets map[string][]string `json:"appliedFacets"`
			Offset int                 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if len(request.Facets) == 0 {
			fmt.Fprint(w, `{"jobPostings":[],"total":21,"facets":[{"facetParameter":"locationCountry","values":[{"descriptor":"India","id":"in"}]}]}`)
			return
		}
		if request.Offset == 20 {
			fmt.Fprint(w, `{"jobPostings":[{"title":"Student Intern","externalPath":"/job/last"}],"total":22}`)
			return
		}
		jobs := []map[string]string{}
		for i := 0; i < 20; i++ {
			jobs = append(jobs, map[string]string{"title": "Engineer", "externalPath": fmt.Sprintf("/job/%d", i)})
		}
		json.NewEncoder(w).Encode(map[string]any{"jobPostings": jobs, "total": 21})
	}))
	defer server.Close()
	source := testSource("workday")
	source.URL = server.URL + "/External"
	posts, err := newCollector().workday(context.Background(), source)
	if err == nil || len(posts) != 0 || !strings.Contains(err.Error(), "count changed") {
		t.Fatalf("changing results marked complete: %+v %v", posts, err)
	}
}
