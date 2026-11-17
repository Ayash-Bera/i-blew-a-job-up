package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func apnaFixture(minimum int, country string, international bool, expiry string) []byte {
	detail := map[string]any{
		"id": 123, "title": "Sales Executive", "description": "Assist customers.", "expiry": expiry,
		"experience_in_years": "Freshers can apply", "min_experience": minimum, "status": 0,
		"employment_type": "Full Time", "public_url": "https://apna.co/job/test-123",
		"organization":           map[string]any{"name": "Example"},
		"address":                map[string]any{"city": map[string]any{"name": "Pune"}},
		"international_job_info": map[string]any{"is_international": international},
	}
	payload, _ := json.Marshal(map[string]any{"jobDetails": detail})
	frame, _ := json.Marshal([]any{1, string(payload)})
	schema := `{"@type":"JobPosting","datePosted":"2026-11-17T10:00:00Z","jobLocation":{"address":{"addressCountry":"` + country + `"}}}`
	return []byte(`<script>self.__next_f.push(` + string(frame) + `)</script><script type="application/ld+json">` + schema + `</script>`)
}

func TestApnaRequiresCurrentDomesticZeroExperience(t *testing.T) {
	for _, test := range []struct {
		name          string
		minimum       int
		country       string
		international bool
		expiry        string
		included      bool
	}{
		{"eligible", 0, "IN", false, "2099-01-01", true},
		{"experience changed", 1, "IN", false, "2099-01-01", false},
		{"international flag", 0, "IN", true, "2099-01-01", false},
		{"foreign schema", 0, "JP", false, "2099-01-01", false},
		{"expired", 0, "IN", false, "2001-01-01", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			post, err := parseApna(apnaFixture(test.minimum, test.country, test.international, test.expiry), testSource("apna"))
			if err != nil || (post != nil) != test.included {
				t.Fatalf("unexpected eligibility: post=%+v err=%v", post, err)
			}
			if post != nil && (post.ValidThrough != "2099-01-01T18:29:59Z" || !strings.Contains(post.Description, "Freshers can apply")) {
				t.Fatalf("source evidence or India expiry changed: %+v", post)
			}
		})
	}
}

func TestApnaParseFailureAndKnownGoneDiffer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gone":
			http.Error(w, "gone", 410)
		default:
			fmt.Fprint(w, `<html>Unexpected page</html>`)
		}
	}))
	defer server.Close()
	c := newCollector()
	posts, closed, err := c.apnaTargets(context.Background(), testSource("apna"), []pageTarget{{URL: server.URL + "/gone", PostingID: "123"}})
	if err != nil || len(posts) != 0 || len(closed) != 1 || closed[0] != "123" {
		t.Fatalf("known gone should be a checked absence: %v", err)
	}
	_, closed, err = c.apnaTargets(context.Background(), testSource("apna"), []pageTarget{{URL: server.URL + "/changed", PostingID: "123"}})
	if err == nil || len(closed) != 0 {
		t.Fatal("unexpected page was treated as a checked absence")
	}
}

func TestInventoryClosureSurvivesPartialFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gone":
			http.Error(w, "gone", http.StatusGone)
		case "/ok":
			w.Write(marriottFixture())
		case "/expired":
			fmt.Fprint(w, strings.ReplaceAll(string(marriottFixture()), "P1-123-0", "P1-expired-0"))
		default:
			fmt.Fprint(w, `<html>Unexpected page</html>`)
		}
	}))
	defer server.Close()
	targets := []marriottTarget{
		{URL: server.URL + "/gone", PostingID: "P1-closed-0", RequisitionID: "777"},
		{URL: server.URL + "/ok", PostingID: "P1-123-0", RequisitionID: "999"},
		{URL: server.URL + "/expired", PostingID: "P1-expired-0", RequisitionID: "999", ValidThrough: "2000-01-01T00:00:00Z"},
		{URL: server.URL + "/changed", PostingID: "P1-unknown-0", RequisitionID: "888"},
	}
	posts, closed, err := newCollector().marriottTargets(context.Background(), testSource("marriott"), targets)
	if err == nil || len(posts) != 1 || len(closed) != 2 || closed[0] != "P1-closed-0" || closed[1] != "P1-expired-0" {
		t.Fatalf("partial fetch lost posting or confirmed closures: posts=%+v closed=%v err=%v", posts, closed, err)
	}
}

func TestInventoryAccessRestrictionStopsQueuedRequests(t *testing.T) {
	for _, code := range []int{403, 429, 481} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "3600")
				http.Error(w, "restricted", code)
			}))
			defer server.Close()
			targets := make([]pageTarget, 40)
			for i := range targets {
				targets[i] = pageTarget{URL: server.URL + "/" + fmt.Sprint(i), PostingID: fmt.Sprint(i)}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			posts, closed, err := newCollector().apnaTargets(ctx, testSource("apna"), targets)
			if !restricted(err) || len(posts) != 0 || len(closed) != 0 || calls.Load() > 4 || !strings.Contains(err.Error(), "Retry-After: 3600") {
				t.Fatalf("restriction did not stop source: calls=%d closed=%v err=%v", calls.Load(), closed, err)
			}
		})
	}
}

func TestInventoryChangedPostingIdentityIsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(marriottFixture())
	}))
	defer server.Close()
	posts, closed, err := newCollector().marriottTargets(context.Background(), testSource("marriott"), []marriottTarget{{URL: server.URL, PostingID: "P1-other-0", RequisitionID: "999"}})
	if err == nil || len(posts) != 0 || len(closed) != 0 {
		t.Fatalf("changed identity was silently accepted: posts=%v closed=%v err=%v", posts, closed, err)
	}
}

func marriottFixture() []byte {
	return []byte(`<script type="application/ld+json">{"@type":"JobPosting","title":"Guest Associate","description":"No related work experience.","identifier":{"value":"P1-123-0"},"datePosted":"2026-11-17T10:00:00Z","url":"https://careers.marriott.com/guest/job/P1-123-0","employmentType":["FULL_TIME"],"jobLocation":[{"address":{"addressCountry":"IN","addressLocality":"Pune"}}]}</script><a data-apply-url="https://example.com/apply?job=999&amp;lang=en">Apply</a><dd class="summary-value reqid">999</dd>`)
}

func TestEmployerIdentityAndExpiry(t *testing.T) {
	target := marriottTarget{URL: "https://careers.marriott.com/guest/job/P1-123-0", RequisitionID: "999"}
	post, err := parseMarriott(marriottFixture(), testSource("marriott"), target)
	if err != nil || post == nil || post.UnderlyingID != "999" || post.PostingID != "P1-123-0" || post.ApplyURL != "https://example.com/apply?job=999&lang=en" {
		t.Fatalf("employer identifiers changed: %+v %v", post, err)
	}
	target.RequisitionID = "different"
	if _, err := parseMarriott(marriottFixture(), testSource("marriott"), target); err == nil {
		t.Fatal("changed requisition was accepted")
	}
	target.RequisitionID = "999"
	target.ValidThrough = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	post, err = parseMarriott(marriottFixture(), testSource("marriott"), target)
	if err != nil || post != nil {
		t.Fatalf("expired employer posting was retained: %+v %v", post, err)
	}
}

func TestWorkdayKeepsInitialTotalOnLaterPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/job/") {
			fmt.Fprint(w, `{"jobPostingInfo":{"jobReqId":"JR1","title":"Student Intern","jobDescription":"University student role","location":"India, Pune"}}`)
			return
		}
		var request struct {
			Facets map[string][]string `json:"appliedFacets"`
			Offset int                 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if len(request.Facets) == 0 {
			fmt.Fprint(w, `{"jobPostings":[],"total":21,"facets":[{"facetParameter":"country","values":[{"descriptor":"India","id":"in"}]}]}`)
			return
		}
		if request.Offset == 20 {
			fmt.Fprint(w, `{"jobPostings":[{"title":"Student Intern","externalPath":"/job/21"}],"total":0}`)
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
	source.Search = "intern"
	posts, err := newCollector().workday(context.Background(), source)
	if err != nil || len(posts) != 1 || posts[0].PostingID != "JR1" {
		t.Fatalf("later-page total truncated collection: %+v %v", posts, err)
	}
}
