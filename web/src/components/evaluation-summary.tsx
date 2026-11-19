"use client";

import { useState } from "react";
import snapshot from "@/data/evaluation-summary.json";
import { safeURL } from "@/lib/query";
import type { Stats } from "@/lib/types";
import { Icon } from "./icons";

type ComparisonRow = (typeof snapshot.current.rows)[number];
type Sample = Pick<typeof snapshot.current, "label" | "sample_size" | "metrics" | "sample_frozen_at">;
type EvidenceFilter = "all" | "differences" | "uncertain";

const percentage = (matches: number, denominator: number) => `${Math.round(matches / denominator * 100)}%`;
const dollars = (value: number | string, precision: number) => new Intl.NumberFormat("en-US", {
  style: "currency", currency: "USD", minimumFractionDigits: precision, maximumFractionDigits: precision,
}).format(Number(value));
const snapshotTime = (value: string) => `${new Intl.DateTimeFormat("en-IN", {
  timeZone: "Asia/Kolkata", day: "numeric", month: "short", year: "numeric", hour: "numeric", minute: "2-digit", second: "2-digit", hour12: true,
}).format(new Date(value))} IST`;
const livePercent = (value: number | null) => value == null ? "Pending" : `${(value * 100).toFixed(1)}%`;

export function EvaluationSummary({ stats, loading }: { stats: Stats | null; loading: boolean }) {
  const [filter, setFilter] = useState<EvidenceFilter>("all");
  const current = snapshot.current;
  const historical = snapshot.historical;
  const cost = snapshot.cost;
  const human = stats?.evaluation;
  const differences = current.rows.filter((row) => !row.matches.joint);
  const uncertain = current.rows.filter((row) => row.reference_certainty !== "high");
  const rows = filter === "differences" ? differences : filter === "uncertain" ? uncertain : current.rows;
  const historicalExamples = historical.rows.filter((row) => !row.matches.joint);
  const marriottCount = current.source_concentration["Marriott International"];
  const remoteCount = current.rows.filter((row) => row.reference.location_status === "remote").length;
  const totalCost = Number(cost.total_usd);
  const humanPending = human && (human.reviewed < human.sample_size || human.joint_accuracy == null);

  return <section className="evaluation-section" aria-labelledby="evaluation-title">
    <div className="section-heading"><div><p className="eyebrow">03 / QUALITY & COST</p><h2 id="evaluation-title">Evidence you can inspect<span>.</span></h2></div></div>
    <div className="evaluation-comparison">
      <p className="section-description">Automated source comparisons check saved tags against separately prepared reference labels. Human review remains a separate step.</p>
      <div className="evaluation-score-grid">
        <ScoreCard sample={historical} variant="historical" />
        <ScoreCard sample={current} variant="current" />
      </div>
      <p className="evaluation-caveat">These are different, disjoint {current.sample_size}-post samples, so the scores do not measure a before-and-after improvement. The current sample contains {marriottCount}/{current.sample_size} Marriott posts and {remoteCount} remote cases. It does not establish performance across the whole index or on remote locations.</p>
      <p className="evaluation-snapshot-meta">{uncertain.length} current references have medium certainty and remain in the score. Work mode and employment type are not scored. Current source text was frozen; vacancy availability was not rechecked.</p>
    </div>

    <div className="evaluation-grid">
      <div className="evaluation-card" aria-busy={loading}>
        <div className="card-title"><Icon name="check" size={20} /><h3>Human tagging audit</h3></div>
        <p>Only signed human reviews count here. The comparison scores above do not count toward this audit.</p>
        <div className="audit-progress"><strong>{loading ? "..." : human ? human.reviewed : "Unavailable"}{!loading && human && <span> / {human.sample_size}</span>}</strong><span>signed posts reviewed (live)</span></div>
        <div className="audit-numbers">
          <div><span>Role accuracy</span><strong>{loading ? "..." : human ? livePercent(human.role_accuracy) : "Unavailable"}</strong></div>
          <div><span>Location accuracy</span><strong>{loading ? "..." : human ? livePercent(human.location_accuracy) : "Unavailable"}</strong></div>
          <div><span>Both correct</span><strong>{loading ? "..." : human ? livePercent(human.joint_accuracy) : "Unavailable"}</strong></div>
        </div>
        {humanPending && <p className="measurement-note">Human review is pending. A final human accuracy result has not been established.</p>}
        {!loading && !human && <p className="measurement-note">Live human-review status is unavailable. No saved count is substituted for it.</p>}
        <p className="evaluation-snapshot-meta">At the comparison freeze: {snapshot.human_review.signed_rows_at_snapshot}/{snapshot.human_review.sample_size} signed reviews.</p>
      </div>

      <div className="evaluation-card">
        <div className="card-title"><Icon name="briefcase" size={20} /><h3>Recorded v4 extraction cost</h3></div>
        <p>Usage-priced estimates include rejected outputs and retries. Hosting, database costs, and later maintenance or review runs are excluded.</p>
        <div className="cost-primary"><strong>{dollars(totalCost * 1000 / cost.published_denominator, 8)}</strong><span>Per 1,000 published posts, using {cost.published_denominator} posts</span></div>
        <div className="cost-row"><span>Per 1,000 attempted post versions ({cost.attempted_versions_denominator})</span><strong>{dollars(totalCost * 1000 / cost.attempted_versions_denominator, 8)}</strong></div>
        <div className="cost-row"><span>Total recorded v4 cost</span><strong>{dollars(totalCost, 7)}</strong></div>
        <p className="measurement-note">{cost.attempts} attempts: {cost.accepted} accepted, {cost.rejected} rejected. {cost.unknown_cost_attempts} attempts have unknown cost. These usage estimates have not been reconciled to a paid bill.</p>
        <p className="evaluation-snapshot-meta">Frozen {snapshotTime(cost.snapshot_at)}. These are recorded snapshot figures, not live spending totals.</p>
      </div>
    </div>

    <div className="evaluation-downloads" aria-label="Download comparison evidence">
      <a className="text-button" href="/evidence/source-comparison.json" download><Icon name="download" size={16} />Full evidence JSON</a>
      <a className="text-button" href="/evidence/v4-comparison.csv" download><Icon name="download" size={16} />Current {current.sample_size}-post CSV</a>
    </div>
    <p className="evaluation-snapshot-meta">Downloads include source links, saved and reference tags, matching results, uncertainty notes, and the recorded cost groups behind these figures.</p>

    <details className="evaluation-evidence">
      <summary>Inspect the current {current.sample_size}-post comparison</summary>
      <p className="section-description">Reference labels were fixed before the saved predictions were opened. Some disagreements involve category boundaries; the notes make those judgments visible.</p>
      <div className="evaluation-filter" role="group" aria-label="Filter current comparison">
        <button className="text-button" aria-pressed={filter === "all"} onClick={() => setFilter("all")}>All ({current.rows.length})</button>
        <button className="text-button" aria-pressed={filter === "differences"} onClick={() => setFilter("differences")}>Differences ({differences.length})</button>
        <button className="text-button" aria-pressed={filter === "uncertain"} onClick={() => setFilter("uncertain")}>Medium certainty ({uncertain.length})</button>
      </div>
      <p className="evaluation-result-count" aria-live="polite">Showing {rows.length} of {current.sample_size} current comparisons.</p>
      <ComparisonTable rows={rows} caption="Current v4 source comparisons" />
    </details>

    <details className="evaluation-evidence">
      <summary>Historical v3 examples: {historicalExamples.length} location differences</summary>
      <p className="section-description">These preserved examples explain the historical result. They belong to a different sample and do not establish the current version&apos;s behavior.</p>
      <ComparisonTable rows={historicalExamples} caption="Historical v3 examples with location differences" historical />
      <div className="evaluation-downloads"><a className="text-button" href="/evidence/v3-examples.csv" download><Icon name="download" size={16} />Historical examples CSV</a></div>
    </details>
  </section>;
}

function ScoreCard({ sample, variant }: { sample: Sample; variant: "historical" | "current" }) {
  return <article className={`evaluation-score-card ${variant}`}>
    <p className="eyebrow">{sample.label} / AUTOMATED COMPARISON</p>
    <h3>{percentage(sample.metrics.joint.matches, sample.metrics.joint.denominator)} <span>both labels match</span></h3>
    <p>{sample.metrics.joint.matches} of {sample.sample_size} posts match on role and location.</p>
    <div className="audit-numbers">
      <div><span>Role agreement</span><strong>{percentage(sample.metrics.role.matches, sample.metrics.role.denominator)}</strong></div>
      <div><span>Location agreement</span><strong>{percentage(sample.metrics.location.matches, sample.metrics.location.denominator)}</strong></div>
    </div>
    <p className="evaluation-snapshot-meta">Sample frozen {snapshotTime(sample.sample_frozen_at)}.</p>
  </article>;
}

function ComparisonTable({ rows, caption, historical = false }: { rows: ComparisonRow[]; caption: string; historical?: boolean }) {
  return <div className="table-shell"><table className="data-table">
    <caption className="sr-only">{caption}</caption>
    <thead><tr><th scope="col">Posting and evidence</th><th scope="col">Reference labels</th><th scope="col">Saved tags</th><th scope="col">Comparison</th></tr></thead>
    <tbody>{rows.map((row) => {
      const url = safeURL(row.source_url);
      return <tr key={row.id}>
        <td><strong>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{row.title}<Icon name="external" size={13} /></a> : row.title}</strong><div>{row.company}</div>
          <details className="evaluation-row-details"><summary>{historical ? "Historical review note" : "Source evidence and review note"}</summary>
            <dl>{!historical && <><dt>Role evidence</dt><dd>{row.evidence.role_evidence || "No duties establish the role."}</dd><dt>Location evidence</dt><dd>{row.evidence.location_evidence || "No location evidence recorded."}</dd><dt>Eligibility evidence</dt><dd>{row.evidence.eligibility_evidence}</dd><dt>Reference certainty</dt><dd>{row.reference_certainty}</dd></>}<dt>Review note</dt><dd>{row.review_note}</dd></dl>
          </details>
        </td>
        <td><Labels value={row.reference} /></td>
        <td><Labels value={row.prediction} /></td>
        <td><span className={`evaluation-match ${row.matches.joint ? "match" : "difference"}`}>{row.matches.joint ? "Both match" : "Difference"}</span><div>Role: {row.matches.role ? "match" : "different"}</div><div>Location: {row.matches.location ? "match" : "different"}</div>{!historical && row.reference_certainty !== "high" && <div>Medium certainty</div>}</td>
      </tr>;
    })}</tbody>
  </table></div>;
}

function Labels({ value }: { value: ComparisonRow["reference"] }) {
  const location = value.cities.length ? value.cities.join(", ") : value.location_status === "region" ? "Region only" : value.location_status === "remote" ? "Remote" : "Unspecified";
  return <div className="evaluation-labels"><strong>{value.role}</strong><span>{location}</span></div>;
}
