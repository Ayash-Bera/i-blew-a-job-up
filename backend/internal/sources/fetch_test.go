package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"firstrole/internal/models"
)

func testSource(kind string) models.Source {
	return models.Source{ID: "test-" + kind, Kind: kind, Board: "example", Company: "Example", URL: "https://example.com", Enabled: true}
}

func TestGreenhouseUnderlyingIdentityAndProspects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jobs":[{"id":1,"internal_job_id":91,"title":"Intern","location":{"name":"Pune"},"content":"&lt;p&gt;Freshers &amp;amp; graduates&lt;/p&gt;","absolute_url":"https://example.com/1"},{"id":2,"internal_job_id":91,"title":"Intern","location":{"name":"Delhi"},"absolute_url":"https://example.com/2"},{"id":3,"internal_job_id":null,"title":"Talent community"}],"meta":{"total":3}}`)
	}))
	defer server.Close()
	c := newCollector()
	c.greenhouseBase = server.URL + "/"
	posts, err := c.greenhouse(context.Background(), testSource("greenhouse"))
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 || posts[0].UnderlyingID != posts[1].UnderlyingID || posts[0].PostingID == posts[1].PostingID {
		t.Fatalf("incorrect job identities: %+v", posts)
	}
	if posts[0].Description != "Freshers & graduates" {
		t.Fatalf("HTML was not decoded: %q", posts[0].Description)
	}
}

func TestLeverPaginationAndPartialFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Query().Get("skip") {
				case "0":
					fmt.Fprint(w, `[{"id":"a","text":"Intern","hostedUrl":"https://example.com/a"},{"id":"b","text":"Trainee","hostedUrl":"https://example.com/b"}]`)
				case "2":
					if fail {
						http.Error(w, "unavailable", http.StatusForbidden)
					} else {
						fmt.Fprint(w, `[{"id":"c","text":"Graduate","hostedUrl":"https://example.com/c"}]`)
					}
				default:
					t.Errorf("unexpected pagination offset %s", r.URL.RawQuery)
				}
			}))
			defer server.Close()
			c := newCollector()
			c.leverBase = server.URL + "/"
			c.pageSize = 2
			results := c.fetch(context.Background(), []models.Source{testSource("lever")})
			want := 3
			if fail {
				want = 2
			}
			if len(results[0].Posts) != want || results[0].Complete == fail {
				t.Fatalf("partial fetch misreported: %+v", results[0])
			}
		})
	}
}

func TestLeverRepeatedPageCannotBeComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":"a","text":"Intern","hostedUrl":"https://example.com/a"}]`)
	}))
	defer server.Close()
	c := newCollector()
	c.leverBase = server.URL + "/"
	c.pageSize = 1
	posts, err := c.lever(context.Background(), testSource("lever"))
	if err == nil || len(posts) != 1 {
		t.Fatalf("repeating pagination accepted: %+v %v", posts, err)
	}
}

func TestCancellationInterruptsRequest(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	c := newCollector()
	c.leverBase = server.URL + "/"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []models.SourceResult, 1)
	go func() { done <- c.fetch(ctx, []models.Source{testSource("lever")}) }()
	<-started
	cancel()
	select {
	case results := <-done:
		if results[0].Complete || results[0].Error == "" {
			t.Fatal("cancellation reported complete")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop fetch")
	}
}

func TestAshbyUnlistedIsExcluded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jobs":[{"id":"a","title":"Intern","location":"Pune","jobUrl":"https://example.com/a","isListed":true},{"id":"b","title":"Private","isListed":false}]}`)
	}))
	defer server.Close()
	c := newCollector()
	c.ashbyBase = server.URL + "/"
	posts, err := c.ashby(context.Background(), testSource("ashby"))
	if err != nil || len(posts) != 1 || posts[0].PostingID != "a" {
		t.Fatalf("unexpected ashby result: %+v %v", posts, err)
	}
}

func TestSmartDetailFailureRemainsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/a"):
			fmt.Fprint(w, `{"id":"a","jobId":"job-a","name":"Intern","postingUrl":"https://example.com/a","jobAd":{"sections":{"qualifications":{"text":"Freshers welcome"}}}}`)
		case strings.HasSuffix(r.URL.Path, "/b"):
			http.Error(w, "missing", http.StatusNotFound)
		default:
			fmt.Fprint(w, `{"content":[{"id":"a"},{"id":"b"}],"totalFound":2}`)
		}
	}))
	defer server.Close()
	c := newCollector()
	c.smartBase = server.URL + "/"
	posts, err := c.smart(context.Background(), testSource("smartrecruiters"))
	if err == nil || len(posts) != 1 || posts[0].UnderlyingID != "job-a" {
		t.Fatalf("partial detail failure was hidden: %+v %v", posts, err)
	}
}

func TestRetryTransientServerError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "temporary", 500)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	c := newCollector()
	c.leverBase = server.URL + "/"
	_, err := c.lever(context.Background(), testSource("lever"))
	if err != nil || requests.Load() != 2 {
		t.Fatalf("transient retry failed: requests=%d err=%v", requests.Load(), err)
	}
}

func TestRegistryRejectsDuplicateIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	data, _ := json.Marshal([]models.Source{testSource("lever"), testSource("lever")})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Registry(path); err == nil {
		t.Fatal("duplicate registry identity accepted")
	}
}
