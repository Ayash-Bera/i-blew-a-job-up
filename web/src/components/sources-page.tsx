"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { formatDate, label, safeURL } from "@/lib/query";
import type { Run, Source, Stats } from "@/lib/types";
import { Icon } from "./icons";
import { EvaluationSummary } from "./evaluation-summary";

export function SourcesPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);
  const [errors, setErrors] = useState<string[]>([]);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    Promise.allSettled([api.stats(controller.signal), api.sources(controller.signal), api.runs(controller.signal)]).then(([statsResult, sourceResult, runResult]) => {
      if (controller.signal.aborted) return;
      setStats(statsResult.status === "fulfilled" ? statsResult.value : null);
      setSources(sourceResult.status === "fulfilled" ? sourceResult.value.sources : []);
      setRuns(runResult.status === "fulfilled" ? runResult.value.runs : []);
      setErrors([statsResult.status === "rejected" ? "Dataset metrics" : "", sourceResult.status === "rejected" ? "Source ledger" : "", runResult.status === "rejected" ? "Refresh history" : ""].filter(Boolean));
      setLoading(false);
    });
    return () => controller.abort();
  }, [revision]);

  return <main id="main-content" className="page-width sources-page">
    <Link href="/" className="back-link"><Icon name="arrow" size={15} style={{ transform: "rotate(180deg)" }} />Back to opportunities</Link>
    <section className="sources-hero"><p className="eyebrow">THE WORK BEHIND THE LISTINGS</p><h1>A source for every role.<br /><span>A record of every check.</span></h1><p>I Blew a Job Up brings public job listings into one searchable index. Here is where they come from, how they qualify, and what the numbers can tell you.</p></section>
    {errors.length > 0 && <div className="notice notice-warning" role="alert"><Icon name="clock" /><div><strong>Some live records could not be loaded.</strong><span> {errors.join(", ")} unavailable. No saved figures are being substituted.</span></div><button className="text-button" onClick={() => { setLoading(true); setRevision((value) => value + 1); }}>Retry <Icon name="refresh" size={15} /></button></div>}
    {stats?.refresh_status === "snapshot_read_only" && <div className="notice notice-warning" role="status"><Icon name="clock" /><div><strong>Saved data preview.</strong><span> Daily refresh is not enabled. Model tagging and the final human audit are not yet complete.</span></div></div>}
    <section className="source-metrics" aria-label="Dataset overview" aria-busy={loading}>
      <Metric title="Published opportunities" value={loading ? "..." : stats?.active_jobs.toLocaleString("en-IN") ?? "Unavailable"} note="Active listings after eligibility checks" />
      <Metric title="Source platforms" value={loading ? "..." : stats?.sources.toLocaleString("en-IN") ?? "Unavailable"} note="Platforms represented in published listings" />
      <Metric title="Tagged listings" value={loading ? "..." : stats?.tagged_jobs.toLocaleString("en-IN") ?? "Unavailable"} note="Roles and locations classified" />
      <Metric title="Latest full refresh" value={loading ? "..." : stats?.last_success_at ? formatDate(stats.last_success_at) : "Not recorded"} note={stats?.last_success_at ? formatDate(stats.last_success_at, true) : "A successful run has not been recorded"} />
    </section>

    <section className="ledger-section" aria-labelledby="ledger-title"><div className="section-heading"><div><p className="eyebrow">01 / PUBLIC SOURCES</p><h2 id="ledger-title">The source ledger<span>.</span></h2></div><a href={api.exportURL} className="text-button"><Icon name="download" size={17} />Export dataset</a></div><p className="section-description">Every source retains its own check status. A collection error is kept visible and never treated as an empty job board.</p>
      <div className="table-shell">{loading ? <p className="table-state" role="status">Loading source records...</p> : sources.length ? <table className="data-table source-table"><caption className="sr-only">Public sources and their latest collection results</caption><thead><tr><th>Source board</th><th>Platform</th><th>Listed roles</th><th>Last successful check</th><th>Status</th></tr></thead><tbody>{sources.map((source) => {
        const url = safeURL(source.url);
        const healthy = ["success", "completed", "healthy", "ok", "active"].includes(source.status.toLowerCase());
        return <tr key={source.id}><td>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{source.name}<Icon name="external" size={13} /></a> : source.name}</td><td>{label(source.kind)}</td><td className="tabular">{source.job_count.toLocaleString("en-IN")}</td><td>{formatDate(source.last_success_at, true)}</td><td><span className={`source-status ${healthy ? "healthy" : "attention"}`}><span />{label(source.status)}</span>{source.last_error && <details className="source-error"><summary>Check note</summary><p>{source.last_error}</p></details>}</td></tr>;
      })}</tbody></table> : <p className="table-state">{errors.includes("Source ledger") ? "The source ledger is temporarily unavailable." : "Source records will appear after the first collection run."}</p>}</div>
    </section>

    <section id="method" className="method-section" aria-labelledby="method-title"><div><p className="eyebrow">02 / INCLUSION & CLASSIFICATION</p><h2 id="method-title">What makes the cut<span>.</span></h2><p className="section-description">The index is for people starting their careers in India. Titles alone do not determine eligibility.</p></div><div className="method-steps"><MethodStep number="01" title="Collect from the original source">Public employer feeds and job boards supply the title, description, location, application link, and check timestamp. A listing must explicitly support an India location or India eligibility.</MethodStep><MethodStep number="02" title="Look for fresher eligibility">Description evidence must support students, freshers, internships, apprenticeships, or zero required experience. A junior title without supporting evidence is insufficient.</MethodStep><MethodStep number="03" title="Merge duplicates, keep the evidence">Posting identifiers, employer requisitions, and canonical job links help identify repeats. Multiple locations are preserved when they belong to one opening.</MethodStep><MethodStep number="04" title="Classify and keep checking">An LLM assigns a role and location using the source text. Missing cities stay unspecified. Refreshes preserve check history, and incomplete source fetches do not remove listings.</MethodStep></div></section>

    <EvaluationSummary stats={stats} loading={loading} />

    <section className="ledger-section" aria-labelledby="refresh-title"><div className="section-heading"><div><p className="eyebrow">04 / REFRESH HISTORY</p><h2 id="refresh-title">A visible paper trail<span>.</span></h2></div><span className="schedule-label"><Icon name="clock" size={15} />Daily target: 06:00 IST</span></div><p className="section-description">Run history is the evidence that collection happened. A configured schedule alone does not establish a successful daily refresh.</p><div className="table-shell">{loading ? <p className="table-state">Loading refresh history...</p> : runs.length ? <table className="data-table"><caption className="sr-only">Latest collection runs</caption><thead><tr><th>Started (IST)</th><th>Status</th><th>Fetched</th><th>Eligible</th><th>Published</th><th>Cached</th><th>Errors</th></tr></thead><tbody>{runs.slice(0, 20).map((run) => <tr key={run.id}><td>{formatDate(run.started_at, true)}</td><td>{label(run.status)}</td><td>{run.fetched.toLocaleString("en-IN")}</td><td>{run.eligible.toLocaleString("en-IN")}</td><td>{run.published.toLocaleString("en-IN")}</td><td>{run.cached.toLocaleString("en-IN")}</td><td>{run.errors}</td></tr>)}</tbody></table> : <p className="table-state">{errors.includes("Refresh history") ? "Refresh history is temporarily unavailable." : "No refresh run has been recorded yet."}</p>}</div></section>
    <div className="bottom-navigation"><Link href="/" className="button button-dark">Find your first role <Icon name="arrow" size={17} /></Link><p>Source dates and check times are shown in India Standard Time.</p></div>
  </main>;
}

function Metric({ title, value, note }: { title: string; value: string; note: string }) {
  return <div className="source-metric"><span>{title}</span><strong>{value}</strong><p>{note}</p></div>;
}

function MethodStep({ number, title, children }: { number: string; title: string; children: React.ReactNode }) {
  return <div className="method-step"><span>{number}</span><div><h3>{title}</h3><p>{children}</p></div></div>;
}
