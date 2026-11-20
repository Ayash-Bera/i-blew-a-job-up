"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { emptyFilters, filterQuery, formatDate, isStale, label, safeURL } from "@/lib/query";
import type { Filters, Job, JobsResponse, Stats } from "@/lib/types";
import { Icon } from "./icons";
import { CompanyLogo } from "./company-logo";

export function JobBoard({ initialFilters }: { initialFilters: Filters }) {
  const [filters, setFilters] = useState<Filters>(initialFilters);
  const [search, setSearch] = useState(initialFilters.q);
  const [data, setData] = useState<JobsResponse | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [statsLoading, setStatsLoading] = useState(true);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [filtersOpen, setFiltersOpen] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    const query = filterQuery(filters, false);
    window.history.replaceState(null, "", query ? `/?${query}` : "/");
    api.jobs(filters, controller.signal).then((result) => {
      setData(result);
      setError("");
    }).catch((reason: Error) => {
      if (!controller.signal.aborted) setError(reason.message);
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [filters, revision]);

  useEffect(() => {
    const controller = new AbortController();
    api.stats(controller.signal).then(setStats).catch(() => {
      if (!controller.signal.aborted) setStats(null);
    }).finally(() => {
      if (!controller.signal.aborted) setStatsLoading(false);
    });
    return () => controller.abort();
  }, [revision]);

  function changeFilter(key: keyof Omit<Filters, "page">, value: string) {
    setLoading(true);
    setFilters((previous) => ({ ...previous, [key]: value, page: 1 }));
  }

  function reset() {
    setSearch("");
    setLoading(true);
    setFilters({ ...emptyFilters });
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const query = String(new FormData(event.currentTarget).get("q") ?? "").trim();
    setSearch(query);
    changeFilter("q", query);
    document.getElementById("opportunities")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function paginate(page: number) {
    setLoading(true);
    setFilters((previous) => ({ ...previous, page }));
    document.getElementById("opportunities")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  const activeFilters = (["q", "role", "city", "type", "mode"] as const).filter((key) => filters[key]);
  const stale = stats ? isStale(stats.last_success_at) : false;
  const partial = stats ? !["success", "completed", "healthy", "ok"].includes(stats.refresh_status.toLowerCase()) : false;
  const snapshot = stats?.refresh_status === "snapshot_read_only";
  const pageBeyondEnd = !!data && data.total > 0 && data.jobs.length === 0 && filters.page > 1;
  const pages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;
  const location = snapshot && !stats?.tagged_jobs ? "City tags pending" : stats ? `${stats.cities.toLocaleString("en-IN")} cities` : "Across India";

  return <main id="main-content">
    <section className="hero page-width" aria-labelledby="hero-title">
      <div className="hero-copy">
        <p className="eyebrow"><span className="eyebrow-line" />THE FIRST STEP, WITH MORE CLARITY</p>
        <h1 id="hero-title">Your first role.<br /><span>A real place to start.</span></h1>
        <p className="hero-description">Fresher jobs and internships across India, gathered from public job listings. {snapshot ? "Preview: model tags and daily refresh are pending." : "Less searching. More possibility."}</p>
        <form className="search-form" role="search" onSubmit={submit}>
          <Icon name="search" size={22} /><label htmlFor="job-search" className="sr-only">Search job title, company, or keyword</label>
          <input id="job-search" name="q" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Role, company, or a fresh start..." autoComplete="off" />
          <button type="submit" className="button button-orange">Find my role <Icon name="arrow" size={18} /></button>
        </form>
        <div className="hero-footnote"><span><Icon name="check" size={14} />Original source links</span><span><Icon name="check" size={14} />No account needed</span><Link href="/sources">See how it works <Icon name="arrow" size={14} /></Link></div>
      </div>
      <aside className="hero-aside" aria-label="Current dataset overview">
        <div className="index-label">THE OPPORTUNITY INDEX <span>INDIA / 01</span></div>
        <div className="hero-count">{stats ? stats.active_jobs.toLocaleString("en-IN") : <span className="count-pending">...</span>}<span className="count-star" aria-hidden="true">*</span></div>
        <p>open doors to your next chapter.</p>
        <div className="hero-mini-stats"><div><strong>{stats ? stats.companies.toLocaleString("en-IN") : "..."}</strong><span>companies</span></div><div><strong>{stats ? stats.cities.toLocaleString("en-IN") : "..."}</strong><span>cities</span></div><span className="big-arrow" aria-hidden="true"><Icon name="arrow" size={54} /></span></div>
        <div className="index-footer"><span className={stats && !stale && !partial ? "status-dot" : "status-dot muted"} />{stats ? snapshot ? "Saved source snapshot" : "Current published listings" : statsLoading ? "Connecting to live data" : "Dataset metrics unavailable"}</div>
      </aside>
    </section>

    <div className="trust-strip"><div className="page-width trust-inner"><span><Icon name="pin" size={15} />{location}</span><span>Fresher-friendly eligibility</span><span>{snapshot ? "Model tags pending" : "Role & city tags"}</span><Link href="/sources"><Icon name="clock" size={15} />{stats?.last_success_at ? `Last refresh ${formatDate(stats.last_success_at, true)}` : "Refresh status pending"}<Icon name="arrow" size={14} /></Link></div></div>

    <section id="opportunities" className="opportunities page-width" aria-labelledby="opportunities-title">
      <div className="section-heading"><div><p className="eyebrow">MAKE YOUR NEXT MOVE</p><h2 id="opportunities-title">Find your starting point<span>.</span></h2></div><a href={api.exportURL} className="text-button export-link"><Icon name="download" size={17} />Export all listings</a></div>
      {(stale || partial) && <div className="notice notice-warning" role="status"><Icon name="clock" size={18} /><div><strong>{snapshot ? "This is a saved source snapshot. Daily refresh is not enabled." : stale ? "Some listings may need a fresh check." : "The latest refresh is incomplete."}</strong><span> Check the original employer post before applying. <Link href="/sources">View source status</Link></span></div></div>}
      {!!stats?.pending_review && <div className="notice notice-neutral" role="status"><Icon name="clock" size={18} /><div><strong>{stats.pending_review.toLocaleString("en-IN")} {snapshot ? "listings" : "eligible posts"} are awaiting model tags.</strong><span>{snapshot ? " Role and city tags are incomplete in this preview." : " These posts are not published in the search results yet."}</span></div></div>}
      <div className="board-layout">
        <aside className={`filter-panel ${filtersOpen ? "is-open" : ""}`} aria-label="Filter jobs">
          <button className="mobile-filter-toggle" aria-expanded={filtersOpen} onClick={() => setFiltersOpen(!filtersOpen)}><span><Icon name="filter" />Filter your search{activeFilters.length > 0 && <span className="filter-count">{activeFilters.length}</span>}</span><Icon name="chevron" /></button>
          <div className="filter-content">
            <div className="filter-heading"><span><Icon name="filter" size={17} />Refine your search</span><button className="reset-button" onClick={reset} disabled={!activeFilters.length}>Reset</button></div>
            <FilterSelect id="filter-role" name="Role" value={filters.role} values={data?.facets.roles ?? []} all="All roles" onChange={(value) => changeFilter("role", value)} />
            <FilterSelect id="filter-city" name="City" value={filters.city} values={data?.facets.cities ?? []} all="Anywhere in India" onChange={(value) => changeFilter("city", value)} />
            <FilterSelect id="filter-type" name="Opportunity type" value={filters.type} values={data?.facets.types ?? []} all="All opportunities" onChange={(value) => changeFilter("type", value)} />
            <FilterSelect id="filter-mode" name="Work mode" value={filters.mode} values={data?.facets.modes ?? []} all="All work modes" onChange={(value) => changeFilter("mode", value)} />
            <div className="filter-note"><span className="note-mark">i</span><p>A fresh start should be for everyone. We include roles with evidence that freshers or students can apply.</p><Link href="/sources#method">Our inclusion criteria <Icon name="arrow" size={14} /></Link></div>
          </div>
        </aside>

        <div className="results" aria-busy={loading}>
          <div className="results-toolbar"><p aria-live="polite">{loading ? "Finding opportunities..." : error ? "Listings unavailable" : <><strong>{data?.total.toLocaleString("en-IN") ?? 0}</strong> {data?.total === 1 ? "opportunity" : "opportunities"}{activeFilters.length ? " match your search" : " to explore"}</>}</p><span className="results-note">Latest discovered first</span></div>
          {!!activeFilters.length && <div className="active-filters">{activeFilters.map((key) => <button key={key} onClick={() => { if (key === "q") setSearch(""); changeFilter(key, ""); }} aria-label={`Remove ${key} filter ${filters[key]}`}>{label(filters[key])}<Icon name="close" size={12} /></button>)}<button className="clear-all" onClick={reset}>Clear all</button></div>}
          {loading ? <div className="loading-list" role="status"><span className="sr-only">Loading listings</span>{[1, 2, 3, 4].map((key) => <div className="skeleton-card" key={key}><div className="skeleton-logo" /><div><div className="skeleton-line short" /><div className="skeleton-line" /><div className="skeleton-line medium" /></div></div>)}</div> : error ? <div className="result-state" role="alert"><span className="state-icon"><Icon name="refresh" size={26} /></span><h3>We couldn&apos;t load the listings.</h3><p>{error}</p><button className="button button-dark" onClick={() => { setLoading(true); setRevision((previous) => previous + 1); }}>Try again <Icon name="refresh" size={16} /></button></div> : !data?.jobs.length ? <div className="result-state"><span className="state-icon"><Icon name="search" size={26} /></span><h3>{pageBeyondEnd ? "This page has moved out of range." : activeFilters.length ? "A little room to explore?" : "The next batch is on its way."}</h3><p>{pageBeyondEnd ? "The result count has changed. Return to the first page to see the current listings." : activeFilters.length ? "No listings match these filters. Try another city or a broader role." : "There are no published opportunities to show yet. Source checks and eligible listings will appear here when ready."}</p>{pageBeyondEnd ? <button className="button button-dark" onClick={() => paginate(1)}>Back to page one <Icon name="arrow" size={16} /></button> : activeFilters.length > 0 && <button className="button button-dark" onClick={reset}>Clear filters <Icon name="arrow" size={16} /></button>}</div> : <>
            <div className="jobs-list">{data.jobs.map((job) => <JobCard key={job.id} job={job} />)}</div>
            <div className="pagination"><p>Showing {(data.page - 1) * data.limit + 1} to {Math.min(data.page * data.limit, data.total)} of {data.total.toLocaleString("en-IN")}</p><div><button className="page-button" aria-label="Previous page" disabled={data.page <= 1} onClick={() => paginate(data.page - 1)}><Icon name="arrow" style={{ transform: "rotate(180deg)" }} /></button><span>Page {data.page} of {pages}</span><button className="page-button" aria-label="Next page" disabled={data.page >= pages} onClick={() => paginate(data.page + 1)}><Icon name="arrow" /></button></div></div>
          </>}
        </div>
      </div>
    </section>
    <section className="method-callout page-width"><div className="callout-symbol" aria-hidden="true"><Icon name="external" size={35} /></div><div><p className="eyebrow">A CLEARER VIEW, FROM THE SOURCE</p><h2>Real posts. Visible evidence.</h2><p>Every listing keeps its original source and fresher eligibility evidence. Explore where the data comes from and what has been checked.</p></div><Link href="/sources" className="button button-outline">Open the source ledger <Icon name="arrow" size={17} /></Link></section>
  </main>;
}

function FilterSelect({ id, name, value, values, all, onChange }: { id: string; name: string; value: string; values: string[]; all: string; onChange: (value: string) => void }) {
  const options = [...new Set(value ? [value, ...values] : values)].sort((left, right) => left.localeCompare(right));
  return <div className="filter-field"><label htmlFor={id}>{name}</label><div className="select-wrap"><select id={id} value={value} onChange={(event) => onChange(event.target.value)}><option value="">{all}</option>{options.map((option) => <option key={option} value={option}>{label(option)}</option>)}</select><Icon name="chevron" size={16} /></div></div>;
}

function JobCard({ job }: { job: Job }) {
  const applyURL = safeURL(job.apply_url);
  const sourceURL = safeURL(job.source_url);
  const pending = job.tagging_method !== "llm";
  const cities = pending ? "City tag pending, see source" : job.cities?.length ? job.cities.join(", ") : job.location_status === "remote" ? "Remote in India" : job.location_status === "region" ? "Regional location, see source" : "Location not specified";
  return <article className="job-card">
    <div className="job-main"><CompanyLogo company={job.company} /><div className="job-summary"><div className="job-company-line"><p className="company-name">{job.company}</p><span className={job.employment_type === "internship" ? "type-badge internship" : "type-badge"}>{label(job.employment_type)}</span></div><h3>{applyURL ? <a href={applyURL} target="_blank" rel="noopener noreferrer">{job.title}</a> : job.title}</h3><div className="job-meta"><span><Icon name="pin" size={14} />{cities}</span>{job.work_mode !== "unspecified" && <span><span className="meta-dot" />{label(job.work_mode)}</span>}<span className="role-tag">{pending ? "Role tag pending" : job.role || "Role not tagged"}</span></div></div><div className="job-action">{applyURL ? <a href={applyURL} target="_blank" rel="noopener noreferrer" className="apply-link" aria-label={`View ${job.title} at ${job.company}`}>View role <Icon name="external" size={15} /></a> : <span className="muted-text">Link unavailable</span>}</div></div>
    <div className="job-evidence-line"><span className="evidence-check"><Icon name="check" size={13} /></span><p>{job.eligibility_evidence || "Eligibility evidence is not recorded for this listing."}</p></div>
    <details className="job-details"><summary><span>Source & details <Icon name="chevron" size={13} /></span><span className="source-name">{job.source}</span></summary><div className="job-detail-content">{job.description_excerpt && <p className="description-excerpt">{job.description_excerpt}</p>}<dl><div><dt>Source checked</dt><dd>{formatDate(job.last_checked_at, true)}</dd></div><div><dt>First discovered</dt><dd>{formatDate(job.first_seen_at)}</dd></div><div><dt>Employer post date</dt><dd>{formatDate(job.published_at)}</dd></div><div><dt>Tagging</dt><dd>{job.tagging_method || "Not recorded"}</dd></div></dl>{sourceURL && <a href={sourceURL} target="_blank" rel="noopener noreferrer" className="source-link">Read the original post <Icon name="external" size={14} /></a>}</div></details>
  </article>;
}
