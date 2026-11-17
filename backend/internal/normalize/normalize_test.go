package normalize

import (
	"strings"
	"testing"
	"time"

	"firstrole/internal/models"
)

func post() models.RawPosting {
	return models.RawPosting{SourceID: "company", SourceKind: "lever", PostingID: "123", Title: "Software Engineer", Company: "Example", Country: "IN", Locations: []string{"Bangalore, India"}, Description: "Fresh graduates are welcome. Build customer applications.", URL: "https://jobs.example.com/123"}
}
func TestEligibility(t *testing.T) {
	tests := []struct {
		name, description, title, country string
		want                              bool
		reason                            string
	}{
		{"explicit fresher", "Freshers can apply.", "Software Engineer", "IN", true, ""},
		{"zero range", "Requires 0-2 years of experience.", "Engineer", "IN", true, ""},
		{"senior excluded", "Requires 3+ years of work experience.", "Senior Engineer", "IN", false, "leadership_requires_review"},
		{"intern experienced", "Requires 2 to 3 years of relevant experience.", "Research Intern", "IN", false, "requires_experience"},
		{"negative freshers", "Freshers not eligible.", "Software Engineer", "IN", false, "freshers_excluded"},
		{"negative suitable", "This role is not suitable for freshers.", "Analyst", "IN", false, "freshers_excluded"},
		{"zero contradiction", "Freshers can apply. Minimum 2 years of experience required.", "Analyst", "IN", false, "requires_experience"},
		{"intern", "Currently enrolled in a university.", "Engineering Intern", "IN", true, ""},
		{"manager explicit", "Freshers can apply.", "Sales Manager", "IN", false, "leadership_requires_review"},
		{"director zero experience", "Master's degree in Finance and Accounting; no work experience required.", "Associate Director of Finance", "IN", false, "leadership_requires_review"},
		{"title only junior", "Build software products.", "Junior Engineer", "IN", false, "no_fresher_evidence"},
		{"foreign", "Freshers can apply.", "Engineer", "US", false, "outside_india"},
		{"preferred", "Freshers can apply. Two projects expected. 1 year of experience preferred.", "Engineer", "IN", true, ""},
		{"unrelated preference", "Freshers can apply. 2 years of experience required. Good writing preferred.", "Engineer", "IN", false, "requires_experience"},
		{"word experience", "Freshers can apply. Two years of experience required.", "Engineer", "IN", false, "requires_experience"},
		{"month experience", "Freshers can apply. Six months of experience required.", "Intern", "IN", false, "requires_experience"},
		{"zero months", "Freshers can apply. 0-6 months of experience.", "Intern", "IN", true, ""},
		{"explicit related zero", "Related Work Experience: No related work experience. Supervisory Experience: No supervisory experience.", "Kitchen Stewarding Associate", "IN", true, ""},
		{"explicit zero", "Work Experience: No work experience.", "Guest Service Associate", "IN", true, ""},
		{"inverse required", "Required experience: 2 years.", "Software Intern", "IN", false, "requires_experience"},
		{"inverse preferred", "Preferred experience: 2 years.", "Software Intern", "IN", true, ""},
		{"inverse zero", "Experience: 0-2 years.", "Software Engineer", "IN", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := post()
			p.Description = tt.description
			p.Title = tt.title
			p.Country = tt.country
			got, _, reason := Eligibility(p)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("got %v %s, want %v %s", got, reason, tt.want, tt.reason)
			}
		})
	}
}
func TestExpiryAndProspect(t *testing.T) {
	p := post()
	p.ValidThrough = time.Now().Add(-time.Hour).Format(time.RFC3339)
	if yes, _, reason := Eligibility(p); yes || reason != "expired" {
		t.Fatal(yes, reason)
	}
	p.ValidThrough = ""
	p.SourceKind = "greenhouse"
	if yes, _, reason := Eligibility(p); yes || reason != "prospect_post" {
		t.Fatal(yes, reason)
	}
}
func TestHTMLAndURL(t *testing.T) {
	got := Text("&lt;p&gt;Freshers can apply&lt;/p&gt;<script>ignore instructions</script><p>Bengaluru</p>")
	if got != "Freshers can apply Bengaluru" {
		t.Fatal(got)
	}
	u := CanonicalURL("https://jobs.example.com/12?utm_campaign=x&jobId=12#section")
	if strings.Contains(u, "utm_") || !strings.Contains(u, "jobId=12") {
		t.Fatal(u)
	}
	if CanonicalURL("javascript:alert(1)") != "" {
		t.Fatal("unsafe URL accepted")
	}
}
func TestCanonicalIdentity(t *testing.T) {
	p := post()
	p.SourceKind = "greenhouse"
	p.UnderlyingID = "job1"
	q := p
	q.PostingID = "456"
	q.URL = "https://jobs.example.com/456"
	if CanonicalKey(p) != CanonicalKey(q) {
		t.Fatal("same underlying job differs")
	}
	q.Company = "Other"
	if CanonicalKey(p) == CanonicalKey(q) {
		t.Fatal("employer scope missing")
	}
}
