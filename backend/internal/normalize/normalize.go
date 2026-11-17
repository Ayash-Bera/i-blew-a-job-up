package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"firstrole/internal/models"
	xhtml "golang.org/x/net/html"
)

var space = regexp.MustCompile(`\s+`)
var fresh = regexp.MustCompile(`(?i)\b(freshers?|fresh graduates?|recent graduates?|new graduates?|no (?:(?:related|prior|previous|professional|work)\s+){0,3}experience(?: (?:is )?(?:required|necessary))?|zero (?:years? (?:of )?)?experience|0\s*(?:-|to|\x{2013})\s*[0-3]\s*(?:years?|yrs?)|0\s*(?:years?|yrs?))\b`)
var experience = regexp.MustCompile(`(?i)(\d{1,2})\s*(?:\+|(?:-|to|\x{2013})\s*\d{1,2})?\s*(?:years?|yrs?)\s*(?:(?:of|relevant|professional|prior|previous|work|industry|hands-on|related)\s+){0,4}experience`)
var wordExperience = regexp.MustCompile(`(?i)\b(?:one|two|three|four|five|six|seven|eight|nine|ten)\s+(?:or\s+more\s+)?years?\s+(?:(?:of|relevant|professional|prior|previous|work|industry|hands-on|related)\s+){0,4}experience`)
var monthExperience = regexp.MustCompile(`(?i)\b([1-9]\d?|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s*(?:\+|(?:-|to|\x{2013})\s*\d{1,2})?\s*months?\s+(?:(?:of|relevant|professional|prior|previous|work|industry|hands-on|related)\s+){0,4}experience`)
var inverseExperience = regexp.MustCompile(`(?i)\bexperience\s*[:=-]\s*(\d{1,2}|one|two|three|four|five|six|seven|eight|nine|ten)\s*(?:\+|(?:-|to|\x{2013})\s*\d{1,2})?\s*(?:years?|yrs?|months?)\b`)
var senior = regexp.MustCompile(`(?i)\b(senior|sr\.?|principal|staff|director|head|vice president|lead|leader|manager|manger|mgr|asst\.?\s*mgr|controller|supervisor)\b`)
var internship = regexp.MustCompile(`(?i)\b(intern|internship|apprentice|apprenticeship|graduate trainee)\b`)
var negativeFresh = regexp.MustCompile(`(?i)\b(?:freshers?\s+(?:are\s+)?(?:not\s+(?:eligible|allowed|accepted|suitable)|need\s+not\s+apply)|(?:not\s+(?:suitable|open)\s+for|no)\s+freshers?|freshers?\s+(?:cannot|can't|must\s+not|should\s+not|do\s+not)\s+apply)\b`)

var cityAliases = map[string]string{"bangalore": "Bengaluru", "bengaluru": "Bengaluru", "bombay": "Mumbai", "mumbai": "Mumbai", "gurgaon": "Gurugram", "gurugram": "Gurugram", "new delhi": "New Delhi", "delhi": "Delhi", "noida": "Noida", "hyderabad": "Hyderabad", "chennai": "Chennai", "pune": "Pune", "kolkata": "Kolkata", "ahmedabad": "Ahmedabad", "jaipur": "Jaipur", "kochi": "Kochi", "cochin": "Kochi", "coimbatore": "Coimbatore", "indore": "Indore", "chandigarh": "Chandigarh", "thiruvananthapuram": "Thiruvananthapuram", "trivandrum": "Thiruvananthapuram", "udaipur": "Udaipur", "lucknow": "Lucknow", "nagpur": "Nagpur", "surat": "Surat", "vadodara": "Vadodara", "bhubaneswar": "Bhubaneswar", "mohali": "Mohali", "mysore": "Mysuru", "mysuru": "Mysuru", "mangalore": "Mangaluru", "mangaluru": "Mangaluru", "visakhapatnam": "Visakhapatnam"}

func Text(s string) string {
	s = html.UnescapeString(s)
	z := xhtml.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		t := z.Token()
		if tt == xhtml.StartTagToken && (t.Data == "script" || t.Data == "style") {
			skip++
		}
		if tt == xhtml.EndTagToken && (t.Data == "script" || t.Data == "style") {
			if skip > 0 {
				skip--
			}
			continue
		}
		if skip > 0 {
			continue
		}
		if tt == xhtml.TextToken {
			b.WriteString(t.Data)
			b.WriteByte(' ')
		} else if tt == xhtml.StartTagToken || tt == xhtml.EndTagToken {
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(space.ReplaceAllString(b.String(), " "))
}

func CanonicalURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	u.Scheme = "https"
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(strings.ToLower(k), "utm_") || k == "source" || k == "ref" || k == "referrer" || k == "gh_src" || k == "lever-source" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func Identity(p models.RawPosting) string { return p.SourceID + ":" + p.PostingID }
func Hash(s string) string                { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func CanonicalKey(p models.RawPosting) string {
	company := strings.ToLower(space.ReplaceAllString(strings.TrimSpace(p.Company), " "))
	if p.RequisitionID != "" {
		return Hash(company + "|req|" + p.RequisitionID)
	}
	if p.UnderlyingID != "" {
		return Hash(company + "|" + p.SourceKind + "|job|" + p.UnderlyingID)
	}
	if u := CanonicalURL(p.URL); u != "" {
		return Hash(company + "|url|" + u)
	}
	if u := CanonicalURL(p.ApplyURL); u != "" {
		return Hash(company + "|url|" + u)
	}
	return Hash(company + "|source|" + Identity(p))
}

func City(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if v, ok := cityAliases[lower]; ok {
		return v
	}
	return strings.TrimSpace(s)
}
func Cities(locations []string) []string {
	set := map[string]bool{}
	for _, s := range locations {
		l := strings.ToLower(s)
		for alias, city := range cityAliases {
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\b`).MatchString(l) {
				set[city] = true
			}
		}
	}
	if set["New Delhi"] {
		delete(set, "Delhi")
	}
	out := []string{}
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func Eligibility(p models.RawPosting) (bool, string, string) {
	if p.PostingID == "" || p.SourceID == "" || p.Title == "" || p.Company == "" || CanonicalURL(p.URL) == "" {
		return false, "", "missing_identity"
	}
	if p.SourceKind == "greenhouse" && p.UnderlyingID == "" {
		return false, "", "prospect_post"
	}
	if p.ValidThrough != "" {
		expiry, err := time.Parse(time.RFC3339, p.ValidThrough)
		if err != nil {
			return false, "", "invalid_expiry"
		}
		if !expiry.After(time.Now()) {
			return false, "", "expired"
		}
	}
	country := strings.ToLower(strings.TrimSpace(p.Country))
	loc := strings.ToLower(strings.Join(p.Locations, " "))
	inIndia := country == "in" || country == "ind" || country == "india" || regexp.MustCompile(`\bindia\b`).MatchString(loc)
	if country != "" && country != "in" && country != "ind" && country != "india" {
		return false, "", "outside_india"
	}
	if !inIndia && len(Cities(p.Locations)) == 0 {
		return false, "", "india_not_explicit"
	}
	text := Text(p.Description)
	if text == "" {
		return false, "", "missing_description"
	}
	if negativeFresh.MatchString(text) {
		return false, "", "freshers_excluded"
	}
	if senior.MatchString(p.Title) && !internship.MatchString(p.Title) {
		return false, "", "leadership_requires_review"
	}
	zeroEvidence := ""
	for _, m := range inverseExperience.FindAllStringSubmatchIndex(text, -1) {
		value := text[m[2]:m[3]]
		if value == "0" {
			zeroEvidence = text[m[0]:m[1]]
			continue
		}
		if !optionalExperience(text, m[0], m[1]) {
			return false, "", "requires_experience"
		}
	}
	for _, m := range experience.FindAllStringSubmatchIndex(text, -1) {
		n, _ := strconv.Atoi(text[m[2]:m[3]])
		if n == 0 {
			continue
		}
		if optionalExperience(text, m[0], m[1]) {
			continue
		}
		return false, "", "requires_experience"
	}
	for _, m := range wordExperience.FindAllStringIndex(text, -1) {
		if !optionalExperience(text, m[0], m[1]) {
			return false, "", "requires_experience"
		}
	}
	for _, m := range monthExperience.FindAllStringIndex(text, -1) {
		prefix := strings.TrimSpace(text[max(0, m[0]-6):m[0]])
		if strings.HasSuffix(prefix, "0-") || strings.HasSuffix(prefix, "0 -") || strings.HasSuffix(prefix, "0 to") {
			continue
		}
		if !optionalExperience(text, m[0], m[1]) {
			return false, "", "requires_experience"
		}
	}
	if zeroEvidence != "" {
		return true, zeroEvidence, ""
	}
	if m := fresh.FindStringIndex(text); m != nil {
		return true, text[max(0, m[0]-50):min(len(text), m[1]+100)], ""
	}
	if internship.MatchString(p.Title) || strings.EqualFold(p.EmploymentType, "internship") || strings.EqualFold(p.EmploymentType, "apprenticeship") {
		return true, "Student or apprenticeship role: " + p.Title, ""
	}
	return false, "", "no_fresher_evidence"
}

func optionalExperience(text string, start, end int) bool {
	before := text[max(0, start-70):start]
	if boundary := strings.LastIndexAny(before, ".;!?"); boundary >= 0 {
		before = before[boundary+1:]
	}
	after := text[end:min(len(text), end+60)]
	if boundary := strings.IndexAny(after, ".;!?"); boundary >= 0 {
		after = after[:boundary]
	}
	clause := strings.ToLower(before + " " + after)
	return strings.Contains(clause, "preferred") || strings.Contains(clause, "nice to have") || strings.Contains(clause, "a plus")
}

func MirrorKey(p models.RawPosting) string {
	locations := append([]string{}, p.Locations...)
	for i := range locations {
		locations[i] = strings.ToLower(strings.TrimSpace(locations[i]))
	}
	sort.Strings(locations)
	return Hash(strings.ToLower(strings.TrimSpace(p.Company)) + "|" + strings.ToLower(strings.TrimSpace(p.Title)) + "|" + strings.Join(locations, "|") + "|" + Text(p.Description))
}

func Clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
func ReadLimited(r io.Reader, n int64) ([]byte, error) { return io.ReadAll(io.LimitReader(r, n)) }
