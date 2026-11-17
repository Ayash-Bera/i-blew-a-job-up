# Public source coverage

`sources.json` is the active source registry. The matching embedded copy at `backend/internal/sources/registry.json` lets the deployed collector run without a filesystem path. Employer ATS feeds discover currently published postings on each scheduled refresh. Public Greenhouse, Lever, and Ashby endpoints need no account or access token.

Marriott uses an explicit inventory of public employer URLs in `marriott-targets.json`, mirrored inside the source package. Each scheduled refresh rechecks every inventory URL, its original posting identity, requisition identity, description, India location, application link, and known expiry. A complete result means every URL in that declared inventory was checked. It does not mean the entire Marriott board was collected. New Marriott URLs require discovery, eligibility review, an inventory update, and a new deployment.

The dataset is concentrated in hospitality because those employers explicitly publish entry-level requirements. Role and city model tags are separate from deterministic eligibility checks. A title containing a student-program term is evidence only when the description does not require prior work experience. Leadership roles require review and are excluded from the initial eligible count. Candidate discovery counts are never reported as accepted job counts.

The verified collection on 18 November 2026 at 01:35 IST returned 1,687 raw observations from 12 completed sources in 27.816 seconds. The strict normalizer retained 226 jobs with 226 distinct job IDs across 11 employers; 191 were from Marriott. Nineteen had a known closing time before 22 November at 23:59 IST, leaving 207 without a known earlier closing time. Of the total, 113 supplied no expiry. These are observed collection results, not a promise of future availability or a tagging-accuracy result. The exact raw export SHA-256 was `d65f48cdce86e3b20f255e4f1bc1789ddf46200dae390388650efe1f55c336cd`.

The collector stops that source on HTTP 403, 429, or 481. It does not retry around an access restriction. Retry-After is preserved in the source error. Other transient failures have bounded retries. Successful rows are preserved when another request fails. Confirmed 404/410 responses and verified expiry or lost eligibility produce explicit source-scoped posting closures, even during partial refreshes. Unexpected HTML, identity changes, and network errors produce partial results rather than invented closure evidence.

`source-candidates.json` holds inactive connectors. Apna returned HTTP 481 during initial research and remains disabled; its cached inventory is not part of the active coverage count. Workday and SmartRecruiters adapters are not enabled in this deployment. Workday needs additional country and changing-pagination review before activation.

To reproduce a live collection from the backend directory:

```sh
go run ./cmd/ingest -source-results-out /tmp/firstrole-source-results.json -snapshot-out /tmp/firstrole-snapshot.json
```

The first output contains the actual source results, including completeness and source errors. The second includes normalization decisions and the public job view. Collection without configured model credentials leaves role and city tags pending; it does not claim model tagging or measured accuracy.

For bounded Marriott discovery, run `python3 scripts/source-marriott-inventory.py coordinator` from the repository root. Search and detail evidence goes under `/tmp/firstrole-marriott-collection`. Cached prior details are research evidence, not a fresh refresh. `candidate-inventory.json` is a review candidate list and never replaces production files automatically. To inspect every collected detail through the same strict normalizer:

```sh
python3 scripts/source-export-evidence.py --all-candidates --output /tmp/firstrole-candidates.json
```

Then run the ingestion command with `-input /tmp/firstrole-candidates.json`. Review accepted observations and their exact evidence before updating both copies of `marriott-targets.json`. Persist the observed posting ID, not a guessed ID or requisition ID, with each URL. Run a new live collection after any inventory update.

Known expiration is enforced, and jobs can close earlier than their stated date. No source guarantees that the current count will remain above 200. Check coverage again before submission, and expand the inventory when it falls. A missing expiry is recorded as unspecified, not inferred as permanent availability.
