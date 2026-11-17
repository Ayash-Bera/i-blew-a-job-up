package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"firstrole/internal/db"
	"firstrole/internal/models"
	"firstrole/internal/normalize"
	"firstrole/internal/tagging"
)

var ErrBusy = errors.New("another refresh is running")

type Fetcher func(context.Context, string) ([]models.SourceResult, error)
type Engine struct {
	Store                db.Store
	Fetch                Fetcher
	RegistryPath         string
	Definitions          []models.Source
	Tagger               *tagging.Client
	MaxTags, Concurrency int
	Timeout              time.Duration
}
type candidate struct {
	Raw       models.RawPosting
	Job       models.Job
	SourceIDs map[string]bool
}

func (e *Engine) Run(ctx context.Context) (run models.Run, retErr error) {
	now := time.Now().UTC()
	run = models.Run{ID: normalize.Hash(fmt.Sprintf("refresh-%d", now.UnixNano()))[:24], StartedAt: now, Status: "running"}
	ttl := 60 * time.Second
	owned, err := e.Store.Acquire(ctx, run.ID, ttl)
	if err != nil {
		return run, err
	}
	if !owned {
		return run, ErrBusy
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := e.Store.Release(c, run.ID); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release refresh lease: %w", err))
		}
	}()
	if err = e.Store.PutRun(ctx, run); err != nil {
		return run, fmt.Errorf("record refresh start: %w", err)
	}
	defer func() {
		finished := time.Now().UTC()
		run.FinishedAt = &finished
		if retErr != nil {
			run.Status = "failed"
			if errors.Is(retErr, context.DeadlineExceeded) || errors.Is(retErr, context.Canceled) {
				run.Status = "timed_out"
			}
		}
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := e.Store.PutRun(c, run); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("record refresh result: %w", err))
		}
	}()
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 240 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func(leaseCtx context.Context) {
		defer close(heartbeatDone)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				if err := e.Store.Renew(leaseCtx, run.ID, ttl); err != nil {
					cancel()
					return
				}
			}
		}
	}(ctx)
	defer func() { cancel(); <-heartbeatDone }()
	s, err := e.Store.Load(ctx, true)
	if err != nil {
		return run, fmt.Errorf("load refresh state: %w", err)
	}
	if s.Observations == nil {
		s.Observations = map[string]models.Observation{}
	}
	fetchCtx, stopFetch := context.WithTimeout(ctx, timeout*2/3)
	results, fetchErr := e.Fetch(fetchCtx, e.RegistryPath)
	stopFetch()
	if fetchErr != nil {
		run.Errors++
	}
	if len(results) == 0 && fetchErr != nil {
		return run, fmt.Errorf("fetch source feeds: %w", fetchErr)
	}
	Reconcile(&s, results, e.Definitions)
	for _, result := range results {
		run.Fetched += len(result.Posts)
		if result.Error != "" || !result.Complete {
			run.Errors++
		}
	}
	candidates := Candidates(s)
	run.Eligible = len(candidates)
	out := make([]models.Job, len(candidates))
	for i, c := range candidates {
		out[i] = c.Job
	}
	if s.InitialCohort == nil && e.Tagger.Ready() && len(candidates) > 0 {
		s.InitialCohort = &models.InitialCohort{StartedAt: time.Now().UTC(), Versions: map[string]string{}}
		for _, candidate := range candidates {
			s.InitialCohort.Versions[candidate.Job.ID] = tagging.CacheKey(candidate.Raw, e.Tagger.Model)
		}
		s.Jobs = out
		if err = e.Store.Save(ctx, s, run.ID); err != nil {
			return run, fmt.Errorf("freeze initial tagging cohort: %w", err)
		}
	}
	var mutex sync.Mutex
	tagCount := 0
	concurrency := e.Concurrency
	if concurrency < 1 {
		concurrency = 4
	}
	work := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range work {
				c := candidates[index]
				job := c.Job
				if e.Tagger != nil {
					key := tagging.CacheKey(c.Raw, e.Tagger.Model)
					record, cacheErr := e.Store.GetTag(ctx, key)
					if cacheErr != nil {
						mutex.Lock()
						run.Errors++
						mutex.Unlock()
					} else if record != nil {
						apply(&job, record.Tags)
						mutex.Lock()
						run.Cached++
						mutex.Unlock()
					} else if e.Tagger.Ready() {
						mutex.Lock()
						allowed := tagCount < e.MaxTags
						if allowed {
							tagCount++
						}
						mutex.Unlock()
						if allowed {
							tags, tagErr := e.Tagger.Tag(ctx, c.Raw, run.ID, job.ID, key, e.Store.PutUsage)
							if tagErr != nil {
								mutex.Lock()
								run.Errors++
								mutex.Unlock()
							} else {
								record := models.TagRecord{Key: key, Model: e.Tagger.Model, PromptVersion: tagging.PromptVersion, Tags: tags, CreatedAt: time.Now().UTC()}
								if err := e.Store.PutTag(ctx, record); err != nil {
									mutex.Lock()
									run.Errors++
									mutex.Unlock()
								} else {
									apply(&job, tags)
								}
							}
						}
					}
				}
				out[index] = job
			}
		}()
	}
schedule:
	for index := range candidates {
		select {
		case work <- index:
		case <-ctx.Done():
			break schedule
		}
	}
	close(work)
	workers.Wait()
	timeoutErr := ctx.Err()
	if timeoutErr != nil {
		finalCtx, stopFinal := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer stopFinal()
		ctx = finalCtx
	}
	s.Jobs = out
	for _, j := range out {
		if j.TaggingMethod == "llm" {
			run.Published++
		}
	}
	s.Mode = "live"
	counts := map[string]int{}
	for i, c := range candidates {
		if out[i].TaggingMethod != "llm" {
			continue
		}
		for id := range c.SourceIDs {
			counts[id]++
		}
	}
	for i := range s.Sources {
		s.Sources[i].JobCount = counts[s.Sources[i].ID]
	}
	usage, err := e.Store.Usage(ctx)
	if err != nil {
		return run, fmt.Errorf("read usage ledger: %w", err)
	}
	s.Costs = Costs(usage, s.Jobs, s.Runs, run.ID)
	finishInitialCohort(&s, candidates, out, usage, e.Tagger)
	run.CostUSD = runCost(usage, run.ID)
	run.Status = "success"
	if run.Errors > 0 {
		run.Status = "partial"
	}
	for _, j := range out {
		if j.TaggingMethod != "llm" && run.Status == "success" {
			run.Status = "awaiting_tags"
		}
	}
	if err = e.Store.Save(ctx, s, run.ID); err != nil {
		return run, fmt.Errorf("commit refresh: %w", err)
	}
	if timeoutErr != nil {
		return run, timeoutErr
	}
	return run, nil
}

func Reconcile(s *models.State, results []models.SourceResult, definitions []models.Source) {
	views := map[string]models.SourceView{}
	for _, v := range s.Sources {
		views[v.ID] = v
	}
	for _, d := range definitions {
		if _, ok := views[d.ID]; !ok {
			views[d.ID] = models.SourceView{ID: d.ID, Name: d.Company, Kind: d.Kind, URL: d.URL, Status: "pending"}
		}
	}
	for _, result := range results {
		v := views[result.SourceID]
		v.ID = result.SourceID
		v.Status = "success"
		v.LastError = result.Error
		if !result.Complete {
			v.Status = "partial"
		}
		if result.Error != "" {
			v.Status = "error"
		}
		if result.Complete && result.Error == "" {
			checked := result.CheckedAt
			v.LastSuccessAt = &checked
		}
		seen := map[string]bool{}
		for _, p := range result.Posts {
			if p.SourceID != result.SourceID {
				continue
			}
			key := normalize.Identity(p)
			seen[key] = true
			previous, exists := s.Observations[key]
			first := p.FetchedAt
			if first.IsZero() {
				first = result.CheckedAt
				p.FetchedAt = first
			}
			if exists {
				first = previous.FirstSeenAt
			}
			eligible, evidence, reason := normalize.Eligibility(p)
			s.Observations[key] = models.Observation{Raw: p, FirstSeenAt: first, Eligible: eligible, Evidence: evidence, Reason: reason}
			if v.Name == "" {
				v.Name = p.Company
				v.Kind = p.SourceKind
				v.URL = p.URL
			}
		}
		if result.Complete && result.Error == "" {
			for key, old := range s.Observations {
				if old.Raw.SourceID == result.SourceID && !seen[key] {
					old.MissingCount++
					s.Observations[key] = old
				}
			}
		}
		for _, postingID := range result.ClosedPostingIDs {
			key := result.SourceID + ":" + postingID
			if previous, ok := s.Observations[key]; ok {
				previous.MissingCount = 2
				previous.Eligible = false
				previous.Reason = "source_closed"
				s.Observations[key] = previous
			}
		}
		views[v.ID] = v
	}
	for _, definition := range definitions {
		if !definition.Enabled {
			v := views[definition.ID]
			v.Status = "disabled"
			v.LastError = "Source is disabled in the active registry"
			views[definition.ID] = v
		}
	}
	s.Sources = []models.SourceView{}
	for _, v := range views {
		s.Sources = append(s.Sources, v)
	}
	sort.Slice(s.Sources, func(i, j int) bool { return s.Sources[i].Name < s.Sources[j].Name })
}

func Candidates(s models.State) []candidate {

	keys := []string{}
	tombstones := []models.RawPosting{}
	for key, o := range s.Observations {
		if authoritative(o.Raw.SourceKind) && (o.MissingCount > 0 || !o.Eligible || expiry(o.Raw.ValidThrough) != nil && !expiry(o.Raw.ValidThrough).After(time.Now())) {
			tombstones = append(tombstones, o.Raw)
		}
		if o.Eligible && o.MissingCount == 0 && !o.Raw.FetchedAt.Before(time.Now().Add(-48*time.Hour)) && (expiry(o.Raw.ValidThrough) == nil || expiry(o.Raw.ValidThrough).After(time.Now())) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	active := keys[:0]
	for _, key := range keys {
		p := s.Observations[key].Raw
		suppressed := false
		if !authoritative(p.SourceKind) {
			for _, closed := range tombstones {
				if samePosting(p, closed) {
					suppressed = true
					break
				}
			}
		}
		if !suppressed {
			active = append(active, key)
		}
	}
	keys = active
	parent := map[string]string{}
	members := map[string][]models.RawPosting{}
	var find func(string) string
	find = func(k string) string {
		if parent[k] == "" {
			parent[k] = k
		}
		if parent[k] != k {
			parent[k] = find(parent[k])
		}
		return parent[k]
	}
	merge := func(a, b string) {
		left, right := find(a), find(b)
		if left == right {
			return
		}
		for _, p := range members[left] {
			for _, q := range members[right] {
				if conflicting(p, q) {
					return
				}
			}
		}
		if left > right {
			left, right = right, left
		}
		parent[right] = left
		members[left] = append(members[left], members[right]...)
		delete(members, right)
	}
	aliases := map[string][]string{}
	mirrors := map[string][]string{}
	for _, key := range keys {
		p := s.Observations[key].Raw
		find(key)
		members[key] = []models.RawPosting{p}
		for _, alias := range postingAliases(p) {
			for _, other := range aliases[alias] {
				merge(key, other)
			}
			aliases[alias] = append(aliases[alias], key)
		}
		mirror := normalize.MirrorKey(p)
		for _, other := range mirrors[mirror] {
			merge(key, other)
		}
		mirrors[mirror] = append(mirrors[mirror], key)
	}
	groups := map[string][]models.Observation{}
	for _, key := range keys {
		groups[find(key)] = append(groups[find(key)], s.Observations[key])
	}
	out := []candidate{}
	previous := map[string]models.Job{}
	for _, j := range s.Jobs {
		previous[j.ID] = j
	}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if authoritative(group[i].Raw.SourceKind) != authoritative(group[j].Raw.SourceKind) {
				return authoritative(group[i].Raw.SourceKind)
			}
			return normalize.Identity(group[i].Raw) < normalize.Identity(group[j].Raw)
		})
		o := group[0]
		raw := o.Raw
		locations := map[string]bool{}
		sourceIDs := map[string]bool{}
		first := o.FirstSeenAt
		last := raw.FetchedAt
		var validThrough *time.Time
		for _, item := range group {
			if authoritative(raw.SourceKind) == authoritative(item.Raw.SourceKind) {
				if date := expiry(item.Raw.ValidThrough); date != nil && (validThrough == nil || date.Before(*validThrough)) {
					validThrough = date
				}
			}
			for _, loc := range item.Raw.Locations {
				locations[loc] = true
			}
			sourceIDs[item.Raw.SourceID] = true
			if item.FirstSeenAt.Before(first) {
				first = item.FirstSeenAt
			}
			if item.Raw.FetchedAt.After(last) {
				last = item.Raw.FetchedAt
			}
		}
		raw.Locations = []string{}
		for loc := range locations {
			raw.Locations = append(raw.Locations, loc)
		}
		sort.Strings(raw.Locations)
		id := normalize.CanonicalKey(raw)
		if prior, ok := previous[id]; ok && prior.FirstSeenAt.Before(first) {
			first = prior.FirstSeenAt
		}
		var published *time.Time
		if t, err := time.Parse(time.RFC3339, raw.PublishedAt); err == nil {
			published = &t
		}
		applyURL := normalize.CanonicalURL(raw.ApplyURL)
		if applyURL == "" {
			applyURL = normalize.CanonicalURL(raw.URL)
		}
		job := models.Job{ID: id, Title: raw.Title, Company: raw.Company, Role: "Unknown", Cities: []string{}, LocationStatus: "unspecified", WorkMode: "unspecified", EmploymentType: "unspecified", Source: raw.SourceKind, SourceURL: normalize.CanonicalURL(raw.URL), ApplyURL: applyURL, EligibilityEvidence: o.Evidence, FirstSeenAt: first, LastCheckedAt: last, PublishedAt: published, ValidThrough: validThrough, TaggingMethod: "pending", DescriptionExcerpt: normalize.Clip(normalize.Text(raw.Description), 400)}
		for sourceID := range sourceIDs {
			job.SourceIDs = append(job.SourceIDs, sourceID)
		}
		sort.Strings(job.SourceIDs)
		out = append(out, candidate{Raw: raw, Job: job, SourceIDs: sourceIDs})
	}
	ids := map[string]int{}
	for _, candidate := range out {
		ids[candidate.Job.ID]++
	}
	for i := range out {
		if ids[out[i].Job.ID] > 1 {
			out[i].Job.ID = normalize.Hash(out[i].Job.ID + "|conflict|" + normalize.Identity(out[i].Raw) + "|" + out[i].Raw.UnderlyingID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Job.ID < out[j].Job.ID })
	return out
}

func apply(j *models.Job, t models.Tags) {
	j.Role = t.Role
	j.Cities = append([]string{}, t.Cities...)
	for i := range j.Cities {
		j.Cities[i] = normalize.City(j.Cities[i])
	}
	sort.Strings(j.Cities)
	j.LocationStatus = t.LocationStatus
	j.WorkMode = t.WorkMode
	j.EmploymentType = t.EmploymentType
	j.TaggingMethod = "llm"
}

func runCost(usage []models.Usage, id string) *float64 {
	var total float64
	for _, u := range usage {
		if u.RunID != id {
			continue
		}
		if u.CostUSD == nil {
			return nil
		}
		total += *u.CostUSD
	}
	return &total
}
func Costs(usage []models.Usage, jobs []models.Job, runs []models.Run, current string) models.Costs {
	c := models.Costs{}
	if len(usage) == 0 {
		return c
	}
	var total float64
	versions := map[string]bool{}
	for _, u := range usage {
		if u.CostUSD == nil {
			return c
		}
		total += *u.CostUSD
		versions[u.JobID+"|"+u.ContentKey] = true
	}
	c.TotalUSD = &total
	if len(versions) > 0 {
		rate := total / float64(len(versions)) * 1000
		c.TaggingPer1000USD = &rate
	}
	return c
}

func Snapshot(results []models.SourceResult, definitions []models.Source) models.State {
	s := db.Empty()
	Reconcile(&s, results, definitions)
	cs := Candidates(s)
	counts := map[string]int{}
	for _, c := range cs {
		s.Jobs = append(s.Jobs, c.Job)
		for id := range c.SourceIDs {
			counts[id]++
		}
	}
	for i := range s.Sources {
		s.Sources[i].JobCount = counts[s.Sources[i].ID]
	}
	s.Mode = "snapshot_read_only"
	return s
}

func expiry(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &t
}

func authoritative(kind string) bool {
	switch kind {
	case "greenhouse", "lever", "ashby", "smartrecruiters", "workday", "marriott":
		return true
	}
	return false
}
func conflicting(a, b models.RawPosting) bool {
	if a.RequisitionID != "" && b.RequisitionID != "" && a.RequisitionID != b.RequisitionID {
		return true
	}
	return a.SourceKind == b.SourceKind && a.UnderlyingID != "" && b.UnderlyingID != "" && a.UnderlyingID != b.UnderlyingID
}
func postingAliases(p models.RawPosting) []string {
	company := strings.ToLower(strings.TrimSpace(p.Company))
	out := []string{normalize.CanonicalKey(p)}
	if u := normalize.CanonicalURL(p.URL); u != "" {
		out = append(out, company+"|url|"+u)
	}
	u := normalize.CanonicalURL(p.ApplyURL)
	specific := u != "" && u == normalize.CanonicalURL(p.URL)
	for _, id := range []string{p.PostingID, p.UnderlyingID, p.RequisitionID} {
		if len(id) >= 3 && strings.Contains(strings.ToLower(u), strings.ToLower(id)) {
			specific = true
		}
	}
	if specific {
		out = append(out, company+"|url|"+u)
	}
	if p.UnderlyingID != "" {
		out = append(out, company+"|"+p.SourceKind+"|"+p.UnderlyingID)
	}
	return out
}
func samePosting(a, b models.RawPosting) bool {
	if !strings.EqualFold(strings.TrimSpace(a.Company), strings.TrimSpace(b.Company)) || conflicting(a, b) {
		return false
	}
	if normalize.MirrorKey(a) == normalize.MirrorKey(b) {
		return true
	}
	for _, left := range postingAliases(a) {
		for _, right := range postingAliases(b) {
			if left == right {
				return true
			}
		}
	}
	return false
}

func finishInitialCohort(s *models.State, candidates []candidate, jobs []models.Job, usage []models.Usage, client *tagging.Client) {
	cohort := s.InitialCohort
	if cohort == nil {
		return
	}
	if cohort.CompletedAt != nil {
		s.Costs.InitialPer1000USD = cohort.Per1000USD
		return
	}
	if client == nil {
		return
	}
	completed := map[string]bool{}
	for i, c := range candidates {
		if jobs[i].TaggingMethod == "llm" && cohort.Versions[c.Job.ID] == tagging.CacheKey(c.Raw, client.Model) {
			completed[c.Job.ID] = true
		}
	}
	if len(completed) != len(cohort.Versions) || len(completed) == 0 {
		return
	}
	total := 0.0
	for _, u := range usage {
		if cohort.Versions[u.JobID] != u.ContentKey || u.CreatedAt.Before(cohort.StartedAt) {
			continue
		}
		if u.CostUSD == nil {
			return
		}
		total += *u.CostUSD
	}
	now := time.Now().UTC()
	rate := total / float64(len(cohort.Versions)) * 1000
	cohort.CompletedAt = &now
	cohort.CostUSD = &total
	cohort.Per1000USD = &rate
	s.Costs.InitialPer1000USD = &rate
}
