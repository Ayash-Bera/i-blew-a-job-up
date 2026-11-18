#!/usr/bin/env python3
"""Report immutable exported usage. Reads local JSON; never contacts a service."""

import argparse
from collections import Counter
from decimal import Decimal
import hashlib
import json
from pathlib import Path
import sys
from audit import cities as canonical_cities

TOKEN_FIELDS = ('input_tokens', 'output_tokens', 'cached_input_tokens', 'thinking_tokens')
RATE_FIELDS = ('input_usd_per_million', 'output_usd_per_million', 'cached_input_usd_per_million')
DEVELOPMENT_MODEL = 'gemini-3.5-flash-lite'
DEVELOPMENT_RATES = dict(zip(RATE_FIELDS, map(Decimal, ('0.30', '2.50', '0.03'))))
MILLION = Decimal(1000000)


def number(value, label):
    if isinstance(value, bool) or not isinstance(value, (int, float, Decimal)):
        raise ValueError(f'{label} must be numeric')
    result = Decimal(str(value))
    if not result.is_finite() or result < 0:
        raise ValueError(f'{label} must be finite and nonnegative')
    return result


def validate_row(row):
    for name in ('id', 'run_id', 'job_id', 'content_key', 'prompt_version', 'model', 'status'):
        if not isinstance(row.get(name), str) or not row[name]:
            raise ValueError(f'usage row needs {name}')
    for name in TOKEN_FIELDS:
        value = row.get(name)
        if value is not None and (type(value) is not int or value < 0):
            raise ValueError(f'{row["id"]}: invalid {name}')
    for part, whole in (('cached_input_tokens', 'input_tokens'), ('thinking_tokens', 'output_tokens')):
        if row.get(part) is not None and row.get(whole) is not None and row[part] > row[whole]:
            raise ValueError(f'{row["id"]}: {part} exceeds {whole}')
    for name in (*RATE_FIELDS, 'cost_usd'):
        if row.get(name) is not None:
            number(row[name], f'{row["id"]}.{name}')


def computed_cost(row, rates=None):
    if any(row.get(name) is None for name in TOKEN_FIELDS):
        return None
    rates = row if rates is None else rates
    if rates.get(RATE_FIELDS[0]) is None or rates.get(RATE_FIELDS[1]) is None:
        return None
    cached = row['cached_input_tokens']
    if cached and rates.get(RATE_FIELDS[2]) is None:
        return None
    input_rate = number(rates[RATE_FIELDS[0]], RATE_FIELDS[0])
    output_rate = number(rates[RATE_FIELDS[1]], RATE_FIELDS[1])
    cached_rate = number(rates.get(RATE_FIELDS[2]) or 0, RATE_FIELDS[2])
    return ((row['input_tokens'] - cached) * input_rate + cached * cached_rate
            + row['output_tokens'] * output_rate) / MILLION


def decimal_json(value):
    return None if value is None else float(value)


def summarize(rows, rates=None):
    complete_counts = []
    missing = []
    unverified_recorded = []
    missing_recorded = []
    recorded = Decimal(0)
    estimates = Decimal(0)
    missing_estimate = []
    totals = {field: 0 for field in TOKEN_FIELDS}
    missing_fields = Counter()
    for row in rows:
        complete = all(row.get(name) is not None for name in TOKEN_FIELDS)
        if complete:
            complete_counts.append(row['id'])
        else:
            missing.append(row['id'])
        for field in TOKEN_FIELDS:
            if row.get(field) is None:
                missing_fields[field] += 1
            else:
                totals[field] += row[field]
        calculated = computed_cost(row)
        supplied = row.get('cost_usd')
        if supplied is None:
            missing_recorded.append(row['id'])
        elif calculated is None:
            unverified_recorded.append(row['id'])
        else:
            supplied = number(supplied, 'cost_usd')
            if abs(supplied - calculated) > max(Decimal('1e-12'), abs(calculated) * Decimal('1e-9')):
                raise ValueError(f'{row["id"]}: recorded cost disagrees with tokens and stored prices')
            recorded += supplied
        if rates is not None:
            estimated = computed_cost(row, rates)
            if estimated is None:
                missing_estimate.append(row['id'])
            else:
                estimates += estimated
    full = None if missing_recorded or unverified_recorded else recorded
    versions = len({(row['job_id'], row['content_key']) for row in rows})
    output = {
        'attempts': len(rows),
        'distinct_attempted_versions': versions,
        'distinct_attempted_jobs': len({row['job_id'] for row in rows}),
        'statuses': dict(sorted(Counter(row['status'] for row in rows).items())),
        'non_ok_attempts': sum(row['status'] != 'ok' for row in rows),
        'complete_usage_attempts': len(complete_counts),
        'missing_usage_attempt_ids': missing,
        'observed_token_subtotals': totals,
        'missing_token_fields': dict(sorted(missing_fields.items())),
        'missing_recorded_cost_attempt_ids': missing_recorded,
        'unverifiable_recorded_cost_attempt_ids': unverified_recorded,
        'verified_recorded_cost_subtotal_usd': decimal_json(recorded),
        'recorded_cost_usd': decimal_json(full),
        'recorded_tagging_per_1000_attempted_versions_usd': decimal_json(
            full * 1000 / versions if full is not None and versions else None),
    }
    if rates is not None:
        estimate = None if missing_estimate else estimates
        output['list_price_estimate'] = {
            'rates_per_million_usd': {name: decimal_json(rate) for name, rate in rates.items()},
            'known_usage_subtotal_usd': decimal_json(estimates),
            'total_usd': decimal_json(estimate),
            'missing_usage_attempt_ids': missing_estimate,
            'per_1000_attempted_versions_usd': decimal_json(
                estimate * 1000 / versions if estimate is not None and versions else None),
            'basis': 'Retrospective Gemini 3.5 Flash Lite list-price estimate; original ledger unchanged',
        }
    return output


def id_array(value, name, allow_empty=False):
    if not isinstance(value, list) or any(not isinstance(item, str) or not item for item in value):
        raise ValueError(f'{name} must be a JSON array of nonempty strings')
    if len(set(value)) != len(value) or (not value and not allow_empty):
        raise ValueError(f'{name} must contain unique IDs and must not be empty')
    return value


def cohort_provenance(data, published_ids, jobs, selected, prompt, model):
    cohort = data.get('initial_cohort')
    for container in ('settings', 'summary', 'state'):
        if cohort is None and isinstance(data.get(container), dict):
            cohort = data[container].get('initial_cohort')
    if cohort is None:
        return {'available': False, 'verified_published_ids': [], 'cached_alias_published_ids': [],
                'unverified_published_ids': published_ids, 'reason': 'Frozen cohort keys not supplied'}
    if not isinstance(cohort, dict) or not isinstance(cohort.get('versions'), dict):
        raise ValueError('invalid initial cohort')
    cache = {}
    for record in data.get('tags', []):
        key = record.get('key')
        if not isinstance(key, str) or key in cache:
            raise ValueError('missing or duplicate tag-cache key')
        cache[key] = record
    success_keys = {row['content_key'] for row in selected if row['status'] == 'ok'}
    success_versions = {(row['job_id'], row['content_key']) for row in selected if row['status'] == 'ok'}
    verified, aliases, missing = [], [], {}
    for job_id in published_ids:
        key = cohort['versions'].get(job_id)
        record = cache.get(key)
        if key is None:
            missing[job_id] = 'Job is outside the supplied frozen cohort'
        elif record is None or record.get('prompt_version') != prompt or record.get('model') != model:
            missing[job_id] = 'Matching frozen tag-cache entry is absent or uses another configuration'
        elif key not in success_keys:
            missing[job_id] = 'No successful attempt for this key in selected first-load runs'
        else:
            tags = record.get('tags', {})
            fields = ('role', 'location_status', 'work_mode', 'employment_type')
            if any(jobs[job_id].get(field) != tags.get(field) for field in fields) or canonical_cities(jobs[job_id].get('cities') or []) != canonical_cities(tags.get('cities') or []):
                missing[job_id] = 'Published fields differ from the frozen tag-cache result'
            else:
                verified.append(job_id)
                if (job_id, key) not in success_versions:
                    aliases.append(job_id)
    return {'available': True, 'cohort_started_at': cohort.get('started_at'),
            'verified_published_ids': verified, 'cached_alias_published_ids': aliases,
            'unverified_published_ids': missing}


def build_report(data, run_ids, published_ids=None, runs=None, prompt='role-city-v2', model='gemini-3.5-flash-lite'):
    run_ids = id_array(run_ids, 'first-load run IDs')
    rows = data.get('usage')
    jobs = data.get('jobs')
    if not isinstance(rows, list) or not isinstance(jobs, list):
        raise ValueError('export must contain usage and jobs arrays')
    seen = set()
    for row in rows:
        validate_row(row)
        if row['id'] in seen:
            raise ValueError('duplicate usage attempt ID')
        seen.add(row['id'])
    selected = [row for row in rows if row['run_id'] in set(run_ids)]
    if any(row['prompt_version'] != prompt for row in selected):
        raise ValueError('selected first-load run contains another prompt version')
    if any(row['model'] != model for row in selected):
        raise ValueError('selected first-load run contains another model')
    run_records = data.get('runs', []) if runs is None else runs
    if isinstance(run_records, dict):
        run_records = run_records.get('runs')
    if not isinstance(run_records, list):
        raise ValueError('run records must be an array or an object containing runs')
    run_lookup = {}
    for run in run_records:
        if not isinstance(run.get('id'), str) or run['id'] in run_lookup:
            raise ValueError('missing or duplicate run record ID')
        run_lookup[run['id']] = run
    observed_runs = {row['run_id'] for row in selected}
    absent_runs = set(run_ids) - observed_runs - set(run_lookup)
    if absent_runs:
        raise ValueError('selected run has neither usage nor a supplied run record: ' + ','.join(sorted(absent_runs)))
    published = {}
    for job in jobs:
        if not isinstance(job.get('id'), str) or job['id'] in published:
            raise ValueError('missing or duplicate job ID')
        published[job['id']] = job
    current_published = {key for key, job in published.items() if job.get('tagging_method') == 'llm'}
    if published_ids is not None:
        published_ids = id_array(published_ids, 'published job IDs', allow_empty=True)
        if set(published_ids) - current_published:
            raise ValueError('declared published IDs are absent or not tagged in the frozen snapshot')
    first = summarize(selected)
    denominator = None if published_ids is None else len(published_ids)
    spend = first['recorded_cost_usd']
    first.update({
        'run_ids': run_ids,
        'prompt_version': prompt,
        'current_snapshot_published_jobs': len(current_published),
        'current_published_jobs_with_selected_attempt': len(current_published & {row['job_id'] for row in selected}),
        'frozen_unique_published_jobs': denominator,
        'published_denominator_basis': 'Explicit caller-supplied IDs, verified tagged in snapshot; cohort provenance asserted by caller' if published_ids is not None else 'Not supplied; headline withheld',
        'recorded_per_1000_published_jobs_usd': spend * 1000 / denominator if spend is not None and denominator else None,
    })
    if published_ids is not None:
        provenance = cohort_provenance(data, published_ids, published, selected, prompt, model)
        first['published_provenance'] = provenance
        if provenance['available']:
            first['published_denominator_basis'] = 'Explicit IDs checked against frozen cohort keys, selected successful attempts, tag-cache configuration, and current published fields'
            if provenance['unverified_published_ids']:
                first['recorded_per_1000_published_jobs_usd'] = None
                first['published_denominator_basis'] += '; headline withheld because some IDs did not verify'
    by_run = {}
    for run_id in run_ids:
        part = summarize([row for row in selected if row['run_id'] == run_id])
        run = run_lookup.get(run_id)
        part['run_record'] = None if run is None else {field: run.get(field) for field in ('status', 'eligible', 'published', 'cached', 'errors', 'cost_usd', 'started_at', 'finished_at')}
        if run is not None:
            for field in ('cached', 'eligible', 'published', 'errors'):
                if type(run.get(field)) is not int or run[field] < 0:
                    raise ValueError(f'{run_id}: invalid run {field}')
            run_cost = run.get('cost_usd')
            if run_cost is not None and part['recorded_cost_usd'] is not None:
                if abs(number(run_cost, 'run cost') - Decimal(str(part['recorded_cost_usd']))) > Decimal('1e-10'):
                    raise ValueError(f'{run_id}: run cost disagrees with selected ledger rows')
        by_run[run_id] = part
    missing_runs = [run_id for run_id in run_ids if run_id not in run_lookup]
    first['cache_hits_across_selected_runs'] = None if missing_runs else sum(run_lookup[run_id]['cached'] for run_id in run_ids)
    first['missing_run_records'] = missing_runs
    development = [row for row in rows if row['prompt_version'] == 'role-city-v1']
    if any(row['model'] != DEVELOPMENT_MODEL for row in development):
        raise ValueError('development estimate rates cannot be applied to another model')
    return {
        'schema_version': 1,
        'model': model,
        'export_checked_at': data.get('summary', {}).get('checked_at'),
        'cash_paid_usd': None,
        'cash_reconciliation': 'Not reconciled against billing or credits',
        'output_tokens_include_thinking': True,
        'development_v1': summarize(development, DEVELOPMENT_RATES),
        'first_load': first,
        'first_load_by_run': by_run,
        'other_attempts_outside_development_and_first_load': sum(row['prompt_version'] != 'role-city-v1' and row['run_id'] not in run_ids for row in rows),
        'limitations': ['Without supplied frozen cohort keys, published cohort provenance is asserted by the caller; snapshot freshness is not independently rechecked', 'Recorded spend represents tagging calls only; hosting and other variable costs are not included', 'Known token and cost subtotals are incomplete whenever missing fields or attempt IDs are listed'],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('export', type=Path)
    parser.add_argument('--run-ids', type=Path, required=True, help='JSON array of explicit first-load run IDs')
    parser.add_argument('--published-job-ids', type=Path, help='Frozen explicit published cohort IDs; required for the headline denominator')
    parser.add_argument('--runs', type=Path, help='Optional run records JSON')
    parser.add_argument('--prompt-version', default='role-city-v2')
    parser.add_argument('--model', default='gemini-3.5-flash-lite')
    args = parser.parse_args()
    paths = {'export': args.export, 'run_ids': args.run_ids, 'published_job_ids': args.published_job_ids, 'runs': args.runs}
    content = {key: path.read_bytes() for key, path in paths.items() if path is not None}
    parsed = {key: json.loads(value) for key, value in content.items()}
    report = build_report(parsed['export'], parsed['run_ids'], parsed.get('published_job_ids'), parsed.get('runs'), args.prompt_version, args.model)
    report['input_sha256'] = {key: hashlib.sha256(value).hexdigest() for key, value in content.items()}
    print(json.dumps(report, indent=2, allow_nan=False))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError) as error:
        print('Cost report rejected: ' + str(error), file=sys.stderr)
        sys.exit(1)
