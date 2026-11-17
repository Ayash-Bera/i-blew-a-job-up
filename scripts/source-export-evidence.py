#!/usr/bin/env python3
"""Export observed public posting facts for the normal ingestion command."""
import argparse
import json
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--input", type=Path, default=Path("/tmp/firstrole-marriott-collection/details.json"))
parser.add_argument("--output", type=Path, default=Path("/tmp/firstrole-source-results.json"))
parser.add_argument("--all-candidates", action="store_true", help="Export all observed details for strict normalization, without claiming a complete inventory refresh.")
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
targets = json.loads((root / "data/marriott-targets.json").read_text())["targets"]
inventory_ids = {target["posting_id"] for target in targets}

rows = []
for record in json.loads(args.input.read_text()):
    if "error" in record or not record.get("posting_id"):
        continue
    if not args.all_candidates and record["posting_id"] not in inventory_ids:
        continue
    rows.append({
        "source_id": "marriott-india-inventory", "source_kind": "marriott",
        "company": "Marriott International", "posting_id": record["posting_id"],
        "underlying_id": record["requisition_id"], "requisition_id": record["requisition_id"],
        "title": record["title"], "locations": record["locations"], "country": record["country"],
        "description": record["description"], "url": record["url"],
        "apply_url": record["metadata"]["applyURL"],
        "employment_type": ", ".join(record["metadata"].get("employmentType", [])),
        "published_at": "", "valid_through": record["valid_through"], "fetched_at": record["checked_at"],
    })
complete = not args.all_candidates and {row["posting_id"] for row in rows} == inventory_ids
error = "" if complete else "Discovery or missing cached records; not a complete inventory refresh"
result = [{"source_id": "marriott-india-inventory", "complete": complete, "posts": rows, "error": error, "checked_at": max((row["fetched_at"] for row in rows), default="1970-01-01T00:00:00Z")}]
args.output.write_text(json.dumps(result))
print(json.dumps({"output": str(args.output), "observed_posts": len(rows), "complete": complete, "unique_requisitions": len({row["requisition_id"] for row in rows})}))
