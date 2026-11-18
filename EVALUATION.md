# Evaluation and cost measurement

No accuracy result exists until a human completes the reference labels. Automated inspection is not the requested hand-check.

## Freeze and sample

Use a separate development pilot to settle the prompt and taxonomy. This project's pilot covered 24 canonical post IDs with 20 provider requests. Four additional posts were inspected while diagnosing rejected outputs. All 28 IDs are excluded through `private/pilot-ids.json`, including posts served through cache reuse. Report rejected-output counts separately from the blind accuracy result. Freeze the model identifier, prompt, schema, and taxonomy before drawing the final sample. Development examples must not enter the final 50.

Export the frozen production dataset with measured predictions. Run:

```text
python3 scripts/audit.py prepare --dataset dataset.json --output private/audit --seed firstrole-2026-11-18 --pilot-ids pilot-ids.json
```

This selects 50 unique posts by deterministic hash ranking and records the sample IDs and file checksums. The destination must be empty; repeated sampling cannot silently overwrite the evaluation. Give only `human-review.csv` to the reviewer. Keep `frozen-predictions.json` separate during labelling.

The human reads the original listing and preserved source evidence, enters the reference role, semicolon-separated cities, location status, eligibility decisions, reviewer name, and an ISO timestamp with time zone. Resolve ambiguous records using the frozen rules. Do not replace difficult examples. If evidence is missing, restore it before claiming a completed review.

Run:

```text
python3 scripts/audit.py score --directory private/audit --output private/audit/results.json
```

The scorer requires exactly 50 distinct reviewed IDs, validates reference tags, refuses changed predictions, normalizes documented city aliases, and reports exact role, city-set plus location-status, and joint matches. It also reports eligibility errors and Wilson 95% intervals. Signing a row is a human action; the program cannot verify the reviewer's identity or quality.

## Role taxonomy

| Role | Boundary |
| --- | --- |
| Software Engineering | Building application, web, mobile, or embedded software |
| Data and Analytics | Analysis, reporting, data engineering, statistics, and modelling |
| Quality Engineering | Software quality assurance and testing |
| Infrastructure and Security | Systems, cloud, reliability, networks, and security engineering |
| Product and Design | Product management, user research, visual and interaction design |
| Sales and Marketing | Sales, business development, promotion, and marketing content |
| Business and Operations | Support, finance, administration, people operations, hospitality, logistics, and routine business processes |
| Other | A clear role outside the defined categories, such as clinical care or non-software trades |
| Unknown | Insufficient evidence to classify the role |

Only cities explicitly tied to the job belong in the city set. Normalize Bangalore to Bengaluru, Gurgaon to Gurugram, Bombay to Mumbai, Cochin to Kochi, Mysore to Mysuru, Mangalore to Mangaluru, Trivandrum to Thiruvananthapuram, and Swai Madhopur to Sawai Madhopur. Comparison ignores case and surrounding spaces, removes duplicate aliases, and preserves distinct cities, including Delhi and New Delhi. Region-only, remote, and unspecified are separate location statuses with empty city sets. Company headquarters and example client locations are not job locations.

When the source location list and predicted city list are both empty, a region or unspecified prediction with missing or unsupported location evidence is stored as unspecified with empty evidence. Supported country evidence is preserved. This fallback never repairs city or remote predictions, invalid location statuses, or role evidence. It is a development-time validation rule, applied before the final audit sample is frozen.

## Costs

Every provider attempt belongs in the usage ledger, including retries and failed calls. A missing usage response means unknown cost, not zero. Preserve input and output tokens, applicable prices, model identifier, run ID, canonical post version, response status, and timestamps. Confirm the ledger against provider billing when available.

Token cost in USD is:

```text
((input_tokens - cached_input_tokens) * input_price_per_million
 + cached_input_tokens * cached_input_price_per_million
 + output_tokens * output_price_per_million) / 1000000
```

For the native Gemini adapter, `output_tokens` already includes candidate and thinking tokens. Do not add `thinking_tokens` to it again. Cached input is a subset of input, and thinking is a subset of output. Missing usage remains unknown; a missing count must not be substituted with zero. Include other billed units separately if the chosen provider has them.

Report these denominators separately:

- Tagging cost per 1,000: tagging spend divided by distinct canonical post versions attempted, multiplied by 1,000. A retry is not a second post.
- Initial usable-post cost per 1,000: all variable first-load costs, including rejected candidates and billable failures, divided by unique eligible posts successfully published in the frozen initial load, multiplied by 1,000.
- Daily maintenance cost per 1,000: daily variable costs divided by distinct active jobs actually checked during that refresh, multiplied by 1,000. Cached unchanged posts belong in this denominator.

Report absolute daily cost, cache hits, new or changed posts, failures, and monthly hosting separately. Distinguish provider list-price cost from cash paid when credits apply. Do not combine development experiments or later maintenance runs into the first-load headline.

### Reproduce the tagging-cost report

Preserve a frozen JSON export containing `usage`, `jobs`, `tags`, and `initial_cohort`. The cohort contains the canonical job-ID to cache-key mapping in `versions`. The reporter also accepts the cohort nested under `settings`, `summary`, or `state`. Preserve every usage row, including development attempts, failed calls, and retries.

Create `private/first-load-run-ids.json` as an explicit JSON array of the frozen v2 first-load run IDs, including its validation run. Create `private/published-job-ids.json` as an explicit array of unique eligible IDs published in that load. Supply the corresponding run records as an array in `private/first-load-runs.json`; these provide cache-hit counts and an independent run-total check.

Run from the repository root:

```text
python3 scripts/cost_report.py private/final-usage.json --run-ids private/first-load-run-ids.json --published-job-ids private/published-job-ids.json --runs private/first-load-runs.json > private/cost-report.json
python3 -m unittest discover -s scripts -p 'test_*.py'
```

The reporter reads local files only and records their SHA-256 hashes. It checks stored token counts and prices against recorded costs, counts every selected attempt, and deduplicates the attempted-version denominator by `(job_id, content_key)`. The published denominator comes from the explicit ID array. With cohort keys supplied, each published ID must match a frozen tag-cache result and a successful request in the selected runs. This includes cached aliases without counting them as additional requests. A missing cohort leaves provenance asserted by the caller; an incomplete supplied cohort withholds the published-post headline.

The `first_load` section reports recorded tagging spend, cost per 1,000 attempted versions, and cost per 1,000 published posts. Missing usage or recorded cost keeps the full amount null while showing known subtotals. Run records are optional, but without them cache-hit totals remain unknown. Hosting and other variable costs must be reported separately before calling this an all-cost first-load figure.

The `development_v1` section preserves the original unknown recorded costs and separately estimates Gemini 3.5 Flash Lite usage at USD 0.30 per million uncached input tokens, USD 2.50 per million output tokens, and USD 0.03 per million cached input tokens. These estimate rates apply only to that model. They do not rewrite historical ledger rows or establish cash paid. Cash remains unreconciled until checked against billing and credits.

Report dataset size and measured cost from the frozen evidence. Final accuracy remains pending until the human review is complete. Report source concentration with the evaluation because a dataset dominated by one employer or role family limits how widely the results generalize.
