import copy
import unittest
import cost_report as report


def attempt(identifier='a1', **changes):
    row = {'id': identifier, 'run_id': 'final-1', 'job_id': 'job-1', 'content_key': 'v2-key',
           'prompt_version': 'role-city-v2', 'model': 'gemini-3.5-flash-lite', 'status': 'ok', 'input_tokens': 1000,
           'output_tokens': 100, 'cached_input_tokens': 100, 'thinking_tokens': 10,
           'input_usd_per_million': 0.30, 'output_usd_per_million': 2.50,
           'cached_input_usd_per_million': 0.03, 'cost_usd': 0.000523}
    row.update(changes)
    return row


def dataset(rows):
    return {'usage': rows, 'jobs': [{'id': 'job-1', 'tagging_method': 'llm'},
                                   {'id': 'alias-2', 'tagging_method': 'llm'},
                                   {'id': 'pending-3', 'tagging_method': 'pending'}]}


def run_record(identifier='final-1', **changes):
    row = {'id': identifier, 'status': 'success', 'eligible': 2, 'published': 2,
           'cached': 1, 'errors': 0, 'cost_usd': None}
    row.update(changes)
    return row


class CostReportTests(unittest.TestCase):
    def test_retry_and_failed_call_spend_with_distinct_denominators(self):
        rows = [attempt(status='invalid_output'), attempt('a2')]
        result = report.build_report(dataset(rows), ['final-1'], ['job-1', 'alias-2'])['first_load']
        self.assertEqual(result['attempts'], 2)
        self.assertEqual(result['non_ok_attempts'], 1)
        self.assertEqual(result['distinct_attempted_versions'], 1)
        self.assertEqual(result['frozen_unique_published_jobs'], 2)
        self.assertAlmostEqual(result['recorded_cost_usd'], 0.001046)
        self.assertAlmostEqual(result['recorded_tagging_per_1000_attempted_versions_usd'], 1.046)
        self.assertAlmostEqual(result['recorded_per_1000_published_jobs_usd'], 0.523)

    def test_missing_usage_preserves_unknown_total_and_known_subtotal(self):
        rows = [attempt(), attempt('a2', status='transport_error', input_tokens=None,
                                  output_tokens=None, cached_input_tokens=None,
                                  thinking_tokens=None, cost_usd=None)]
        result = report.build_report(dataset(rows), ['final-1'], ['job-1'])['first_load']
        self.assertEqual(result['missing_usage_attempt_ids'], ['a2'])
        self.assertIsNone(result['recorded_cost_usd'])
        self.assertIsNone(result['recorded_per_1000_published_jobs_usd'])
        self.assertAlmostEqual(result['verified_recorded_cost_subtotal_usd'], 0.000523)

    def test_development_estimate_never_changes_ledger_or_final_total(self):
        rows = [attempt(), attempt('dev', prompt_version='role-city-v1', run_id='pilot',
                                   input_usd_per_million=None, output_usd_per_million=None,
                                   cached_input_usd_per_million=None, cost_usd=None)]
        original = copy.deepcopy(rows)
        result = report.build_report(dataset(rows), ['final-1'], ['job-1'])
        self.assertEqual(rows, original)
        self.assertIsNone(result['development_v1']['recorded_cost_usd'])
        self.assertAlmostEqual(result['development_v1']['list_price_estimate']['total_usd'], 0.000523)
        self.assertAlmostEqual(result['first_load']['recorded_cost_usd'], 0.000523)
        self.assertIsNone(result['cash_paid_usd'])

    def test_cached_and_thinking_counts_are_required_and_not_added_twice(self):
        result = report.build_report(dataset([attempt()]), ['final-1'])['first_load']
        self.assertAlmostEqual(result['recorded_cost_usd'], 0.000523)
        result = report.build_report(dataset([attempt(thinking_tokens=None)]), ['final-1'])['first_load']
        self.assertIsNone(result['recorded_cost_usd'])
        self.assertEqual(result['unverifiable_recorded_cost_attempt_ids'], ['a1'])

    def test_headline_requires_explicit_verified_published_ids(self):
        result = report.build_report(dataset([attempt()]), ['final-1'])['first_load']
        self.assertIsNone(result['recorded_per_1000_published_jobs_usd'])
        self.assertEqual(result['current_snapshot_published_jobs'], 2)
        with self.assertRaisesRegex(ValueError, 'absent or not tagged'):
            report.build_report(dataset([attempt()]), ['final-1'], ['pending-3'])

    def test_wrong_price_duplicate_attempt_and_wrong_prompt_are_rejected(self):
        for rows, expected in (([attempt(cost_usd=0.9)], 'disagrees'),
                               ([attempt(), attempt()], 'duplicate'),
                               ([attempt(prompt_version='role-city-v1')], 'another prompt')):
            with self.subTest(expected=expected), self.assertRaisesRegex(ValueError, expected):
                report.build_report(dataset(rows), ['final-1'])

    def test_run_records_allow_zero_attempt_cache_run_and_report_cache_count(self):
        runs = [run_record(cost_usd=0.000523), run_record('cache-only', cached=2, cost_usd=0)]
        result = report.build_report(dataset([attempt()]), ['final-1', 'cache-only'], ['job-1', 'alias-2'], runs)
        self.assertEqual(result['first_load']['cache_hits_across_selected_runs'], 3)
        self.assertEqual(result['first_load_by_run']['cache-only']['recorded_cost_usd'], 0)
        with self.assertRaisesRegex(ValueError, 'neither usage'):
            report.build_report(dataset([attempt()]), ['final-1', 'missing'])

    def test_inconsistent_or_negative_counts_and_incomplete_development(self):
        for field, value in (('cached_input_tokens', 1001), ('thinking_tokens', 101),
                             ('input_tokens', -1), ('output_tokens', True)):
            with self.subTest(field=field), self.assertRaises(ValueError):
                report.build_report(dataset([attempt(**{field: value})]), ['final-1'])
        rows = [attempt(), attempt('dev', run_id='pilot', prompt_version='role-city-v1',
                                  cached_input_tokens=None, cost_usd=None)]
        result = report.build_report(dataset(rows), ['final-1'])
        self.assertIsNone(result['development_v1']['list_price_estimate']['total_usd'])

    def test_frozen_cohort_proves_cached_alias_without_extra_attempt(self):
        data = dataset([attempt()])
        tags = {'role': 'Other', 'cities': ['Mumbai'], 'location_status': 'city',
                'work_mode': 'onsite', 'employment_type': 'internship'}
        for job in data['jobs'][:2]:
            job.update(tags)
        data['initial_cohort'] = {'versions': {'job-1': 'v2-key', 'alias-2': 'v2-key'}}
        data['tags'] = [{'key': 'v2-key', 'prompt_version': 'role-city-v2',
                         'model': 'gemini-3.5-flash-lite', 'tags': tags}]
        result = report.build_report(data, ['final-1'], ['job-1', 'alias-2'])['first_load']
        self.assertEqual(result['published_provenance']['verified_published_ids'], ['job-1', 'alias-2'])
        self.assertEqual(result['published_provenance']['cached_alias_published_ids'], ['alias-2'])
        self.assertEqual(result['distinct_attempted_versions'], 1)
        self.assertAlmostEqual(result['recorded_per_1000_published_jobs_usd'], 0.2615)
        data['jobs'][1]['role'] = 'Unknown'
        result = report.build_report(data, ['final-1'], ['job-1', 'alias-2'])['first_load']
        self.assertIsNone(result['recorded_per_1000_published_jobs_usd'])
        self.assertIn('alias-2', result['published_provenance']['unverified_published_ids'])

    def test_model_changes_cannot_reuse_development_rates(self):
        with self.assertRaisesRegex(ValueError, 'another model'):
            report.build_report(dataset([attempt(model='different')]), ['final-1'])
        rows = [attempt(), attempt('dev', prompt_version='role-city-v1', run_id='pilot', model='different')]
        with self.assertRaisesRegex(ValueError, 'another model'):
            report.build_report(dataset(rows), ['final-1'])
        rows = [attempt(model='different'), attempt('dev', prompt_version='role-city-v1', run_id='pilot', model='different')]
        with self.assertRaisesRegex(ValueError, 'another model'):
            report.build_report(dataset(rows), ['final-1'], model='different')

    def test_provenance_accepts_city_aliases_but_rejects_different_cities(self):
        cases = [('Bangalore', 'Bengaluru', True), ('Cochin', 'Kochi', True),
                 ('Swai Madhopur', 'Sawai Madhopur', True),
                 ('Bangalore', 'Chennai', False), ('Delhi', 'New Delhi', False)]
        for cached_city, published_city, matches in cases:
            with self.subTest(cached=cached_city, published=published_city):
                data = dataset([attempt()])
                tags = {'role': 'Other', 'cities': [cached_city], 'location_status': 'city',
                        'work_mode': 'onsite', 'employment_type': 'internship'}
                data['jobs'][0].update(tags)
                data['jobs'][0]['cities'] = [published_city]
                data['initial_cohort'] = {'versions': {'job-1': 'v2-key'}}
                data['tags'] = [{'key': 'v2-key', 'prompt_version': 'role-city-v2',
                                'model': 'gemini-3.5-flash-lite', 'tags': tags}]
                first = report.build_report(data, ['final-1'], ['job-1'])['first_load']
                provenance = first['published_provenance']
                if matches:
                    self.assertEqual(provenance['verified_published_ids'], ['job-1'])
                    self.assertEqual(provenance['unverified_published_ids'], {})
                    self.assertAlmostEqual(first['recorded_per_1000_published_jobs_usd'], 0.523)
                else:
                    self.assertEqual(provenance['verified_published_ids'], [])
                    self.assertIn('job-1', provenance['unverified_published_ids'])
                    self.assertIsNone(first['recorded_per_1000_published_jobs_usd'])


if __name__ == '__main__':
    unittest.main()
