import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { safeURL } from "../src/lib/query.ts";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const snapshot = JSON.parse(read("../src/data/evaluation-summary.json"));
const aliases = {
  bangalore: "bengaluru", gurgaon: "gurugram", bombay: "mumbai", cochin: "kochi",
  mysore: "mysuru", mangalore: "mangaluru", trivandrum: "thiruvananthapuram",
  "swai madhopur": "sawai madhopur",
};
const cities = (values) => [...new Set(values.map((city) => {
  const normalized = city.trim().toLowerCase();
  return aliases[normalized] ?? normalized;
}))].sort();

function calculatedMatches(row) {
  const role = row.reference.role === row.prediction.role;
  const location = row.reference.location_status === row.prediction.location_status
    && JSON.stringify(cities(row.reference.cities)) === JSON.stringify(cities(row.prediction.cities));
  return { role, location, joint: role && location };
}

function decimalUnits(value, scale = snapshot.cost.usage_cost_decimal_places) {
  const [whole, fraction = ""] = value.split(".");
  assert.match(whole, /^\d+$/);
  assert.match(fraction, /^\d*$/);
  assert.ok(fraction.length <= scale);
  return BigInt(whole) * 10n ** BigInt(scale) + BigInt(fraction.padEnd(scale, "0"));
}

function ratePerThousand(total, denominator) {
  const divisor = 10n ** BigInt(snapshot.cost.usage_cost_decimal_places) * BigInt(denominator);
  const rounded = (total * 1000n * 100000000n + divisor / 2n) / divisor;
  return (rounded / 100000000n).toString() + "." + (rounded % 100000000n).toString().padStart(8, "0");
}

function parseCSV(text) {
  const rows = [];
  let row = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (quoted) {
      if (char === '"' && text[i + 1] === '"') { cell += '"'; i++; }
      else if (char === '"') quoted = false;
      else cell += char;
    } else if (char === '"') {
      assert.equal(cell, "");
      quoted = true;
    } else if (char === ",") {
      row.push(cell); cell = "";
    } else if (char === "\n") {
      row.push(cell); rows.push(row); row = []; cell = "";
    } else if (char !== "\r") cell += char;
  }
  assert.equal(quoted, false);
  if (cell || row.length) { row.push(cell); rows.push(row); }
  return rows;
}

test("saved comparisons recompute from reference and predicted labels", () => {
  for (const sample of [snapshot.historical, snapshot.current]) {
    assert.equal(sample.rows.length, sample.sample_size);
    assert.equal(new Set(sample.rows.map((row) => row.id)).size, sample.sample_size);
    const totals = { role: 0, location: 0, joint: 0 };
    for (const row of sample.rows) {
      const matches = calculatedMatches(row);
      assert.deepEqual(row.matches, matches, sample.label + " row " + row.row);
      for (const field of Object.keys(totals)) totals[field] += Number(matches[field]);
      for (const labels of [row.reference, row.prediction]) {
        assert.equal(labels.cities.length > 0, labels.location_status === "city");
        assert.equal(cities(labels.cities).length, labels.cities.length);
      }
    }
    for (const field of Object.keys(totals)) {
      assert.deepEqual(sample.metrics[field], { matches: totals[field], denominator: sample.rows.length });
    }
  }
  assert.equal(snapshot.historical.metrics.joint.matches / snapshot.historical.sample_size, 0.94);
  assert.equal(snapshot.current.metrics.joint.matches / snapshot.current.sample_size, 0.9);
  assert.equal(snapshot.historical.metrics.role.matches / snapshot.historical.sample_size, 1);
  assert.equal(snapshot.current.metrics.location.matches / snapshot.current.sample_size, 1);
});

test("sample limitations and unsigned human review remain explicit", () => {
  const historicalIDs = new Set(snapshot.historical.rows.map((row) => row.id));
  const overlap = snapshot.current.rows.filter((row) => historicalIDs.has(row.id)).length;
  assert.equal(snapshot.comparison.overlap, overlap);
  assert.equal(overlap, 0);
  assert.equal(snapshot.comparison.disjoint_samples, true);
  assert.equal(snapshot.review_kind, "automated_source_comparison");
  assert.deepEqual(snapshot.human_review, { signed_rows_at_snapshot: 0, sample_size: 50, complete: false });
  assert.equal(snapshot.current.rows.filter((row) => row.company === "Marriott International").length, 39);
  assert.equal(snapshot.current.rows.filter((row) => row.reference.location_status === "remote").length, 0);
  assert.deepEqual(snapshot.current.uncertain_rows, snapshot.current.rows.filter((row) => row.reference_certainty !== "high").map((row) => row.row));
  assert.equal(snapshot.current.uncertain_rows.length, 8);
  assert.equal(snapshot.comparison.work_mode_scored, false);
  assert.equal(snapshot.comparison.employment_type_scored, false);
  assert.equal(snapshot.comparison.live_vacancies_rechecked, false);
});

test("recorded usage independently reproduces both cost rates with decimal arithmetic", () => {
  const cost = snapshot.cost;
  const total = cost.usage_records.reduce((sum, record) => sum + decimalUnits(record.cost_usd), 0n);
  assert.equal(total, decimalUnits(cost.total_usd));
  assert.equal(cost.total_usd, "0.2088703");
  for (const [field, outcome] of [["accepted", "accepted"], ["rejected", "rejected"]]) {
    assert.equal(cost[field], cost.usage_records.filter((row) => row.outcome === outcome).reduce((sum, row) => sum + row.attempts, 0));
  }
  assert.equal(cost.attempts, cost.accepted + cost.rejected);
  assert.equal(cost.attempts, 241);
  assert.equal(cost.unknown_cost_attempts, cost.usage_records.reduce((sum, row) => sum + row.unknown_cost_attempts, 0));
  assert.equal(cost.published_denominator, 236);
  assert.equal(cost.attempted_versions_denominator, 225);
  assert.equal(ratePerThousand(total, cost.published_denominator), cost.recorded_per_1000_published_usd);
  assert.equal(ratePerThousand(total, cost.attempted_versions_denominator), cost.recorded_per_1000_attempted_versions_usd);
  assert.equal((Number(cost.total_usd) * 1000 / cost.published_denominator).toFixed(8), cost.recorded_per_1000_published_usd);
  assert.equal((Number(cost.total_usd) * 1000 / cost.attempted_versions_denominator).toFixed(8), cost.recorded_per_1000_attempted_versions_usd);
  assert.equal(cost.billing_reconciled, false);
  assert.deepEqual(cost.excludes, ["Hosting", "Database", "Later maintenance and review runs"]);
});

test("downloadable evidence matches the app snapshot without private source fields", () => {
  assert.deepEqual(JSON.parse(read("../public/evidence/source-comparison.json")), snapshot);
  const forbidden = new Set(["source_text", "description", "raw_html", "reviewer", "reviewer_name", "signature", "api_key", "access_token", "authorization"]);
  function inspect(value) {
    if (typeof value === "string") {
      assert.doesNotMatch(value, /\/home\/|\/tmp\/|file:\/\/|[A-Z]:\\/);
      assert.doesNotMatch(value, /[\u2013\u2014]/);
    } else if (Array.isArray(value)) value.forEach(inspect);
    else if (value && typeof value === "object") {
      for (const [key, entry] of Object.entries(value)) {
        assert.ok(!forbidden.has(key), key);
        inspect(entry);
      }
    }
  }
  inspect(snapshot);
  for (const row of [...snapshot.current.rows, ...snapshot.historical.rows]) {
    assert.ok(safeURL(row.source_url));
    const url = new URL(row.source_url);
    assert.ok(!["localhost", "127.0.0.1", "[::1]"].includes(url.hostname));
    assert.equal(url.username, "");
    assert.equal(url.password, "");
  }
  for (const record of snapshot.provenance.input_records) {
    assert.ok(!record.name.startsWith("/"));
    assert.ok(!record.name.split("/").includes(".."));
    assert.match(record.sha256, /^[a-f0-9]{64}$/);
  }
});

test("CSV exports retain complete rows, evidence, and safe spreadsheet cells", () => {
  assert.deepEqual(parseCSV('name,note\r\none,"a,b and ""quote"""\r\n'), [["name", "note"], ["one", 'a,b and "quote"']]);
  for (const [file, expected] of [
    ["v4-comparison.csv", snapshot.current.rows],
    ["v3-examples.csv", snapshot.historical.rows.filter((row) => !row.matches.joint)],
  ]) {
    const [header, ...rows] = parseCSV(read("../public/evidence/" + file));
    assert.equal(rows.length, expected.length);
    for (let i = 0; i < rows.length; i++) {
      assert.equal(rows[i].length, header.length);
      const record = Object.fromEntries(header.map((key, index) => [key, rows[i][index]]));
      assert.equal(record.id, expected[i].id);
      assert.equal(record.source_url, expected[i].source_url);
      assert.equal(record.role_evidence, expected[i].evidence.role_evidence);
      assert.equal(record.location_evidence, expected[i].evidence.location_evidence);
      assert.equal(record.eligibility_evidence, expected[i].evidence.eligibility_evidence);
      assert.equal(record.joint_match, String(expected[i].matches.joint));
      assert.equal(record.reference_certainty, expected[i].reference_certainty);
      for (const value of rows[i]) assert.doesNotMatch(value.trimStart(), /^[=+@-]/);
    }
  }
});
