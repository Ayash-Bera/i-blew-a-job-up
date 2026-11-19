import csv
import json
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from audit import cities, prepare, score, safe_cell


class AuditTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.dataset = self.root / "dataset.json"
        self.jobs = [{"id": str(i), "title": "Intern", "company": "Test employer",
                      "role": "Software Engineering", "cities": ["Bengaluru"],
                      "location_status": "city", "source_url": "https://example.test/job/" + str(i),
                      "last_checked_at": "2026-11-18T00:00:00Z", "tagging_method": "llm"}
                     for i in range(70)]
        self.dataset.write_text(json.dumps(self.jobs))
        self.out = self.root / "audit"

    def complete_reviews(self):
        path = self.out / "human-review.csv"
        with path.open(newline="") as handle:
            reader = csv.DictReader(handle)
            fields, rows = reader.fieldnames, list(reader)
        for row in rows:
            row.update(gold_role="Software Engineering", gold_cities="Bangalore",
                       gold_location_status="city", india_eligible="yes", fresher_eligible="yes",
                       reviewer="Test fixture", reviewed_at="2026-11-18T06:00:00+05:30")
        rows[0]["gold_role"] = "Other"
        with path.open("w", newline="") as handle:
            writer = csv.DictWriter(handle, fieldnames=fields)
            writer.writeheader()
            writer.writerows(rows)

    def test_blind_sample_is_reproducible_and_excludes_pilot(self):
        pilot = self.root / "pilot.json"
        pilot.write_text(json.dumps([str(i) for i in range(20)]))
        first = prepare(self.dataset, self.out, "fixed-seed", pilot)
        second = prepare(self.dataset, self.root / "second", "fixed-seed", pilot)
        self.assertEqual(first["ids"], second["ids"])
        self.assertFalse(set(first["ids"]) & set(first["pilot_ids"]))
        self.assertEqual(50, len(first["ids"]))
        with self.assertRaisesRegex(ValueError, "empty"):
            prepare(self.dataset, self.out, "changed-seed", pilot)

    def test_incomplete_review_cannot_produce_accuracy(self):
        prepare(self.dataset, self.out, "seed", None)
        with self.assertRaisesRegex(ValueError, "incomplete"):
            score(self.out)

    def test_known_error_and_city_alias_score_correctly(self):
        prepare(self.dataset, self.out, "seed", None)
        self.complete_reviews()
        result = score(self.out)
        self.assertEqual(49, result["joint"]["correct"])
        self.assertEqual(50, result["location"]["correct"])
        self.assertLess(result["joint"]["wilson_95_interval"][0], .98)

    def test_city_aliases_preserve_distinct_city_sets(self):
        equivalents = [("Bangalore", "Bengaluru"), ("Gurgaon", "Gurugram"),
                       ("Bombay", "Mumbai"), ("Cochin", "Kochi"), ("Mysore", "Mysuru"),
                       ("Mangalore", "Mangaluru"), ("Trivandrum", "Thiruvananthapuram"),
                       ("Swai Madhopur", "Sawai Madhopur")]
        for alias, canonical in equivalents:
            with self.subTest(alias=alias):
                self.assertEqual(cities(" " + alias.upper() + " "), cities([canonical]))
                self.assertEqual(cities(alias + ";" + canonical), cities([canonical]))
        self.assertNotEqual(cities("Delhi"), cities("New Delhi"))
        self.assertEqual(cities("Delhi;New Delhi"), ["delhi", "new delhi"])

    def test_scoring_accepts_aliases_but_rejects_a_different_city(self):
        canonical = ["Bengaluru", "Gurugram", "Mumbai", "Kochi", "Mysuru",
                     "Mangaluru", "Thiruvananthapuram", "Sawai Madhopur", "New Delhi"]
        aliases = "Bangalore;Gurgaon;Bombay;Cochin;Mysore;Mangalore;Trivandrum;Swai Madhopur;New Delhi"
        for job in self.jobs:
            job["cities"] = canonical
        self.dataset.write_text(json.dumps(self.jobs))
        prepare(self.dataset, self.out, "alias-score", None)
        self.complete_reviews()
        path = self.out / "human-review.csv"
        with path.open(newline="") as handle:
            reader = csv.DictReader(handle)
            fields, rows = reader.fieldnames, list(reader)
        for row in rows:
            row["gold_cities"] = aliases
        different_id = rows[1]["id"]
        rows[1]["gold_cities"] = aliases.replace("New Delhi", "Delhi")
        with path.open("w", newline="") as handle:
            writer = csv.DictWriter(handle, fieldnames=fields)
            writer.writeheader()
            writer.writerows(rows)
        result = score(self.out)
        self.assertEqual(result["location"]["correct"], 49)
        mismatches = [row["id"] for row in result["results"] if not row["location_correct"]]
        self.assertEqual(mismatches, [different_id])

    def test_changed_predictions_rejected(self):
        prepare(self.dataset, self.out, "seed", None)
        self.complete_reviews()
        with (self.out / "frozen-predictions.json").open("a") as handle:
            handle.write(" ")
        with self.assertRaisesRegex(ValueError, "changed"):
            score(self.out)

    def test_spreadsheet_formula_is_neutralized(self):
        self.assertEqual("'=SUM(A1)", safe_cell("=SUM(A1)"))
        self.assertEqual("'  @SUM(A1)", safe_cell("  @SUM(A1)"))
        self.assertEqual("Intern", safe_cell("Intern"))

    def prepare_strict(self):
        for job in self.jobs:
            job.update(source_text="Build software. Fresh graduates can apply.",
                       original_locations=["Bangalore, India"], original_country="IN",
                       original_employment_type="Intern", content_key="test-" + job["id"],
                       prompt_version="role-city-v4")
        self.dataset.write_text(json.dumps(self.jobs))
        prepare(self.dataset, self.out, "strict-seed", None, strict_source=True)

    def test_strict_sample_requires_full_source(self):
        with self.assertRaisesRegex(ValueError, "Full source"):
            prepare(self.dataset, self.out, "seed", None, strict_source=True)

    def test_strict_review_checks_source_timestamp_and_bom(self):
        self.prepare_strict()
        self.complete_reviews()
        with self.assertRaisesRegex(ValueError, "predates"):
            score(self.out)
        path = self.out / "human-review.csv"
        with path.open(newline="") as handle:
            reader = csv.DictReader(handle)
            fields, rows = reader.fieldnames, list(reader)
        for row in rows:
            row["reviewed_at"] = datetime.now(timezone.utc).isoformat()
        def save():
            with path.open("w", newline="", encoding="utf-8-sig") as handle:
                writer = csv.DictWriter(handle, fieldnames=fields)
                writer.writeheader()
                writer.writerows(rows)
        save()
        result = score(self.out)
        self.assertEqual(result["joint"]["correct"], 49)
        self.assertEqual(len(result["completed_review_sha256"]), 64)
        rows[0]["original_locations"] = '["Mumbai, India"]'
        save()
        with self.assertRaisesRegex(ValueError, "source fields changed"):
            score(self.out)


if __name__ == "__main__":
    unittest.main()
