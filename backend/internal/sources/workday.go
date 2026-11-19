package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"firstrole/internal/models"
)

type workdayFacet struct {
	Parameter  string         `json:"facetParameter"`
	Descriptor string         `json:"descriptor"`
	ID         string         `json:"id"`
	Values     []workdayFacet `json:"values"`
}

func indiaFacet(facets []workdayFacet) map[string][]string {
	for _, facet := range facets {
		for _, value := range facet.Values {
			if strings.EqualFold(value.Descriptor, "India") && facet.Parameter != "" && value.ID != "" {
				return map[string][]string{facet.Parameter: {value.ID}}
			}
		}
		if nested := indiaFacet(facet.Values); len(nested) != 0 {
			return nested
		}
	}
	return nil
}

type workdayPage struct {
	Total  *int           `json:"total"`
	Facets []workdayFacet `json:"facets"`
	Jobs   []struct {
		Title string `json:"title"`
		Path  string `json:"externalPath"`
	} `json:"jobPostings"`
}

var studentTitle = regexp.MustCompile(`(?i)\b(intern|internship|apprentice|apprenticeship|graduate|trainee|fresher|student)\b`)

func (c *collector) workday(ctx context.Context, source models.Source) ([]models.RawPosting, error) {
	boardURL, err := url.Parse(source.URL)
	if err != nil || boardURL.Host == "" {
		return nil, fmt.Errorf("invalid workday board URL")
	}
	site := strings.Trim(boardURL.Path, "/")
	if strings.Contains(site, "/") {
		return nil, fmt.Errorf("workday source URL must identify the career site root")
	}
	base := boardURL.Scheme + "://" + boardURL.Host + "/wday/cxs/" + url.PathEscape(source.Board) + "/" + url.PathEscape(site)
	var initial workdayPage
	body := map[string]any{"appliedFacets": map[string][]string{}, "limit": 20, "offset": 0, "searchText": ""}
	if err := c.request(ctx, http.MethodPost, base+"/jobs", body, &initial); err != nil {
		return nil, err
	}
	facets := indiaFacet(initial.Facets)
	if len(facets) == 0 {
		return nil, fmt.Errorf("workday response has no explicit India location facet")
	}
	body["appliedFacets"] = facets
	body["searchText"] = source.Search
	paths := []string{}
	seen := map[string]bool{}
	expected, count := -1, 0
	finished := false
	for page := 0; page < c.maxPages; page++ {
		body["offset"] = page * 20
		var payload workdayPage
		if err := c.request(ctx, http.MethodPost, base+"/jobs", body, &payload); err != nil {
			return nil, err
		}
		if payload.Jobs == nil || (page == 0 && payload.Total == nil) {
			return nil, fmt.Errorf("missing workday posting list or initial count")
		}
		if expected < 0 {
			expected = *payload.Total
		}
		if payload.Total != nil && *payload.Total > 0 && *payload.Total != expected {
			return nil, fmt.Errorf("workday result count changed during pagination")
		}
		for _, job := range payload.Jobs {
			if job.Path == "" || seen[job.Path] {
				return nil, fmt.Errorf("missing or repeated workday posting")
			}
			seen[job.Path] = true
			if studentTitle.MatchString(job.Title) {
				paths = append(paths, job.Path)
			}
		}
		count += len(payload.Jobs)
		if count > expected {
			return nil, fmt.Errorf("workday returned %d postings above initial count %d", count, expected)
		}
		if count >= expected {
			finished = true
			break
		}
		if len(payload.Jobs) == 0 {
			return nil, fmt.Errorf("workday pagination ended at %d of %d", count, expected)
		}
	}
	if !finished {
		return nil, fmt.Errorf("workday pagination safety limit reached")
	}
	posts := []models.RawPosting{}
	for _, path := range paths {
		var detail struct {
			Info struct {
				ID          string   `json:"jobReqId"`
				Title       string   `json:"title"`
				Description string   `json:"jobDescription"`
				Location    string   `json:"location"`
				Additional  []string `json:"additionalLocations"`
				StartDate   string   `json:"startDate"`
				TimeType    string   `json:"timeType"`
				URL         string   `json:"externalUrl"`
				Country     struct {
					Descriptor string `json:"descriptor"`
				} `json:"country"`
			} `json:"jobPostingInfo"`
		}
		if err := c.request(ctx, http.MethodGet, base+path, nil, &detail); err != nil {
			return posts, err
		}
		info := detail.Info
		if info.ID == "" || info.Title == "" || info.Description == "" {
			return posts, fmt.Errorf("incomplete workday posting detail")
		}
		post := raw(source)
		post.PostingID, post.UnderlyingID, post.RequisitionID = info.ID, info.ID, info.ID
		post.Title, post.Description = info.Title, plain(info.Description)
		post.Country = info.Country.Descriptor
		post.Locations = unique(append([]string{info.Location}, info.Additional...)...)
		post.URL = info.URL
		if post.URL == "" {
			post.URL = source.URL + path
		}
		post.ApplyURL, post.EmploymentType = post.URL, info.TimeType
		posts = append(posts, post)
	}
	return posts, nil
}
