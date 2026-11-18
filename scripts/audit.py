"""Reproducible blind sampling and strict scoring of human reference labels."""

import argparse
import csv
import hashlib
import json
import math
from datetime import datetime, timezone
from pathlib import Path

ROLES = {
    "Software Engineering", "Data and Analytics", "Quality Engineering",
    "Infrastructure and Security", "Product and Design", "Sales and Marketing",
    "Business and Operations", "Other", "Unknown",
}
STATUSES = {"city", "region", "remote", "unspecified"}
GOLD_FIELDS = ["gold_role", "gold_cities", "gold_location_status", "india_eligible",
               "fresher_eligible", "reviewer", "reviewed_at", "notes"]


def digest(value):
    return hashlib.sha256(value).hexdigest()


def load_jobs(path):
    data = json.loads(Path(path).read_text())
    jobs = data if isinstance(data, list) else data["jobs"]
    if len({j["id"] for j in jobs}) != len(jobs):
        raise ValueError("Dataset contains repeated canonical IDs")
    if not all(j.get("tagging_method") == "llm" for j in jobs):
        raise ValueError("The evaluation dataset must contain measured LLM predictions only")
    return jobs


def safe_cell(value):
    value = str(value or "")
    if value.lstrip().startswith(("=", "+", "-", "@")) or value.startswith(("\t", "\r")):
        return "'" + value
    return value


def write_csv(path, rows, fields):
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows({k: safe_cell(row.get(k, "")) for k in fields} for row in rows)


def prepare(dataset, destination, seed, pilot_ids):
    jobs = load_jobs(dataset)
    exclusions = set(json.loads(Path(pilot_ids).read_text())) if pilot_ids else set()
    population = [j for j in jobs if j["id"] not in exclusions]
    if len(population) < 50:
        raise ValueError("At least 50 non-pilot model-tagged posts are required")
    ranked = sorted(population, key=lambda j: digest((seed + ":" + j["id"]).encode()))
    selected = ranked[:50]
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    if any(destination.iterdir()):
        raise ValueError("Audit destination must be empty to preserve frozen predictions")
    rows = [{
        "id": job["id"], "title": job["title"], "company": job["company"],
        "source_url": job["source_url"], "source_text": job.get("source_text", job.get("description_excerpt", "")),
        "last_checked_at": job["last_checked_at"],
    } for job in selected]
    write_csv(destination / "human-review.csv", rows,
              ["id", "title", "company", "source_url", "source_text", "last_checked_at"] + GOLD_FIELDS)
    predictions = json.dumps(selected, indent=2, ensure_ascii=False).encode()
    (destination / "frozen-predictions.json").write_bytes(predictions)
    manifest = {
        "created_at": datetime.now(timezone.utc).isoformat(), "seed": seed,
        "selection": "Lowest SHA-256(seed + ':' + canonical ID), without replacement",
        "population": len(jobs), "eligible_sample_population": len(population),
        "pilot_ids": sorted(exclusions), "sample_size": 50,
        "dataset_sha256": digest(Path(dataset).read_bytes()),
        "predictions_sha256": digest(predictions), "ids": [j["id"] for j in selected],
        "human_review_complete": False,
    }
    (destination / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    return manifest


def cities(value):
    aliases = {"bangalore": "bengaluru", "gurgaon": "gurugram", "bombay": "mumbai",
               "cochin": "kochi", "mysore": "mysuru", "mangalore": "mangaluru",
               "trivandrum": "thiruvananthapuram", "swai madhopur": "sawai madhopur"}
    parts = value if isinstance(value, list) else value.split(";")
    return sorted({aliases.get(v.strip().casefold(), v.strip().casefold()) for v in parts if v.strip()})


def interval(correct, count):
    p = correct / count
    z = 1.959963984540054
    denominator = 1 + z * z / count
    center = (p + z * z / (2 * count)) / denominator
    margin = z * math.sqrt(p * (1-p)/count + z*z/(4*count*count)) / denominator
    return [round(center-margin, 6), round(center+margin, 6)]


def score(directory):
    directory = Path(directory)
    manifest = json.loads((directory / "manifest.json").read_text())
    raw = (directory / "frozen-predictions.json").read_bytes()
    if digest(raw) != manifest["predictions_sha256"]:
        raise ValueError("Frozen predictions changed after sampling")
    predictions = {j["id"]: j for j in json.loads(raw)}
    with (directory / "human-review.csv").open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    if len(rows) != 50 or len(predictions) != 50 or len({r["id"] for r in rows}) != 50:
        raise ValueError("Exactly 50 distinct human reviews are required")
    if set(predictions) != {r["id"] for r in rows} or set(manifest["ids"]) != set(predictions):
        raise ValueError("Review IDs differ from the frozen sample")
    results = []
    for row in rows:
        job = predictions[row["id"]]
        if row["gold_role"] not in ROLES or row["gold_location_status"] not in STATUSES:
            raise ValueError(f"{row['id']}: incomplete or invalid reference tags")
        if row["india_eligible"] not in {"yes", "no"} or row["fresher_eligible"] not in {"yes", "no"}:
            raise ValueError(f"{row['id']}: eligibility must be reviewed as yes or no")
        if not row["reviewer"].strip() or not row["reviewed_at"].strip():
            raise ValueError(f"{row['id']}: human reviewer and timestamp required")
        timestamp = datetime.fromisoformat(row["reviewed_at"].replace("Z", "+00:00"))
        if timestamp.tzinfo is None:
            raise ValueError(f"{row['id']}: reviewer timestamp needs a time zone")
        if row["gold_location_status"] == "city" and not cities(row["gold_cities"]):
            raise ValueError(f"{row['id']}: city status requires at least one city")
        if row["gold_location_status"] != "city" and cities(row["gold_cities"]):
            raise ValueError(f"{row['id']}: non-city status requires an empty city set")
        role_ok = row["gold_role"] == job["role"]
        location_ok = cities(row["gold_cities"]) == cities(job["cities"]) and row["gold_location_status"] == job["location_status"]
        results.append({"id": row["id"], "role_correct": role_ok, "location_correct": location_ok,
                        "joint_correct": role_ok and location_ok,
                        "eligibility_correct": row["india_eligible"] == "yes" and row["fresher_eligible"] == "yes"})
    report = {"sample_size": 50, "results": results,
              "reviewed_at": datetime.now(timezone.utc).isoformat(),
              "predictions_sha256": manifest["predictions_sha256"]}
    for metric in ["role", "location", "joint", "eligibility"]:
        correct = sum(r[metric + "_correct"] for r in results)
        report[metric] = {"correct": correct, "accuracy": correct / 50,
                          "wilson_95_interval": interval(correct, 50)}
    report["reviewers"] = sorted({row["reviewer"] for row in rows})
    return report


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    sample = commands.add_parser("prepare")
    sample.add_argument("--dataset", required=True)
    sample.add_argument("--output", required=True)
    sample.add_argument("--seed", required=True)
    sample.add_argument("--pilot-ids")
    evaluate = commands.add_parser("score")
    evaluate.add_argument("--directory", required=True)
    evaluate.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        if args.command == "prepare":
            result = prepare(args.dataset, args.output, args.seed, args.pilot_ids)
            print(f"Prepared {result['sample_size']} blind review rows; human review pending")
        else:
            result = score(args.directory)
            Path(args.output).write_text(json.dumps(result, indent=2) + "\n")
            print(f"Scored 50 reviewed posts: {result['joint']['correct']}/50 joint matches")
    except (ValueError, KeyError, OSError) as error:
        parser.exit(1, f"Audit stopped: {error}\n")


if __name__ == "__main__":
    main()
