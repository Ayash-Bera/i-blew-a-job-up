# Evaluation and cost measurement

No accuracy result exists until a human completes the reference labels. Automated inspection is not the requested hand-check.

## Freeze and sample

Use 20 development posts to settle the prompt and taxonomy. Record their canonical IDs in a JSON array. Freeze the model identifier, prompt, schema, and taxonomy before drawing the final sample. Development examples must not enter the final 50.

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

Only cities explicitly tied to the job belong in the city set. Normalize Bangalore to Bengaluru and Gurgaon to Gurugram. Preserve all explicit cities. Region-only, remote, and unspecified are separate location statuses with empty city sets. Company headquarters and example client locations are not job locations.

## Costs

Every provider attempt belongs in the usage ledger, including retries and failed calls. A missing usage response means unknown cost, not zero. Preserve input and output tokens, applicable prices, model identifier, run ID, canonical post version, response status, and timestamps. Confirm the ledger against provider billing when available.

Token cost in USD is `(input_tokens * input_price_per_million + output_tokens * output_price_per_million) / 1000000`. Include other billed units if the chosen provider has them.

Report these denominators separately:

- Tagging cost per 1,000: tagging spend divided by distinct canonical post versions attempted, multiplied by 1,000. A retry is not a second post.
- Initial usable-post cost per 1,000: all variable first-load costs, including rejected candidates and billable failures, divided by unique eligible posts successfully published in the frozen initial load, multiplied by 1,000.
- Daily maintenance cost per 1,000: daily variable costs divided by distinct active jobs actually checked during that refresh, multiplied by 1,000. Cached unchanged posts belong in this denominator.

Report absolute daily cost, cache hits, new or changed posts, failures, and monthly hosting separately. Distinguish provider list-price cost from cash paid when credits apply. Do not combine development experiments or later maintenance runs into the first-load headline.

Dataset size, final tagging accuracy, and actual cost figures remain pending until measured. Source concentration must be reported with the evaluation because a dataset dominated by one employer or role family limits how widely the results generalize.
