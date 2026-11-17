import test from "node:test";
import assert from "node:assert/strict";
import { emptyFilters, filterQuery, safeURL, isStale, formatDate } from "../src/lib/query.ts";

test("filter values round trip without losing punctuation and reset omits old filters", () => {
  const query = new URLSearchParams(filterQuery({ ...emptyFilters, q: " C++ intern ", city: "Bengaluru", page: 2 }));
  assert.equal(query.get("q"), "C++ intern");
  assert.equal(query.get("city"), "Bengaluru");
  assert.equal(query.get("page"), "2");
  assert.equal(query.get("limit"), "20");
  assert.equal(filterQuery(emptyFilters, false), "");
});

test("job links accept public web URLs and reject active content or embedded credentials", () => {
  assert.equal(safeURL("https://careers.example.org/jobs/123"), "https://careers.example.org/jobs/123");
  for (const value of ["javascript:alert(1)", "data:text/html,hello", "file:///etc/passwd", "https://user:pass@example.org", "invalid", ""]) assert.equal(safeURL(value), undefined);
});

test("freshness does not pass missing timestamps as current", () => {
  const now = Date.parse("2026-11-18T12:00:00Z");
  assert.equal(isStale(null, now), true);
  assert.equal(isStale("invalid", now), true);
  assert.equal(isStale("2026-11-17T05:59:00Z", now), true);
  assert.equal(isStale("2026-11-18T11:00:00Z", now), false);
});

test("timestamps render in India time and missing dates stay unknown", () => {
  assert.equal(formatDate("2026-11-18T00:30:00Z", true), "18 Nov 2026, 06:00 am IST");
  assert.equal(formatDate(null), "Not recorded");
});
