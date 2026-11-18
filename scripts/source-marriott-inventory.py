#!/usr/bin/env python3
"""Collect public employer pages and preserve exact entry-level evidence."""
import concurrent.futures
import datetime
import html
import json
import math
from pathlib import Path
import re
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
OUT = Path("/tmp/firstrole-marriott-collection")
OUT.mkdir(exist_ok=True)
STOP = threading.Event()
SCHEMA = re.compile(r"<script[^>]*type=[\"']application/ld\+json[\"'][^>]*>(.*?)</script>", re.S | re.I)
NO_EXPERIENCE = re.compile(r"(?:No related work experience|No work experience|No prior (?:work )?experience|current college or university student|currently (?:a )?(?:college|university) student)", re.I)


def fetch(url):
    if STOP.is_set():
        raise RuntimeError("Stopped after access restriction")
    time.sleep(0.35)
    try:
        request = urllib.request.Request(url, headers={"User-Agent": "IBlewAJobUp/1.0 public employer job listings"})
        with urllib.request.urlopen(request, timeout=35) as response:
            return response.read().decode()
    except urllib.error.HTTPError as error:
        if error.code in (403, 429, 481):
            STOP.set()
        raise


def search(word, page=1):
    query = urllib.parse.urlencode({"filter[country][0]": "India", "keyword": word, "page_number": page})
    text = fetch("https://careers.marriott.com/jobs?" + query)
    marker = "window.__PRELOAD_STATE__ = "
    state = json.JSONDecoder().raw_decode(text[text.index(marker) + len(marker):])[0]
    return state["jobSearch"]


def detail(job):
    url = "https://careers.marriott.com/" + job["originalURL"].lstrip("/")
    try:
        text = fetch(url)
        schemas = [json.loads(x) for x in SCHEMA.findall(text)]
        schema = next(x for x in schemas if x.get("@type") == "JobPosting")
        description = html.unescape(re.sub(r"<[^>]+>", " ", schema["description"]))
        description = re.sub(r"\s+", " ", description).strip()
        evidence = NO_EXPERIENCE.search(description)
        countries = [x.get("address", {}).get("addressCountry") for x in schema.get("jobLocation", [])]
        fields = {x["cfKey"]: x.get("value") for x in job.get("customFields", [])}
        expiry = fields.get("cf_publish_job_end_date", "")
        now = datetime.datetime.now(datetime.timezone.utc).isoformat()
        qualified = evidence is not None and "IN" in countries and (not expiry or expiry > now) and fields.get("cf_external_posting_status") == "Posted"
        return {
            "url": url, "posting_id": schema["identifier"]["value"], "requisition_id": job["requisitionID"],
            "title": schema["title"], "description": description, "country": "IN" if "IN" in countries else "",
            "locations": [x.get("address", {}).get("addressLocality") for x in schema.get("jobLocation", []) if x.get("address", {}).get("addressCountry") == "IN"],
            "eligibility_evidence": evidence.group(0) if evidence else "", "valid_through": expiry,
            "source_fields_eligible": qualified, "checked_at": now, "metadata": job,
        }
    except Exception as error:
        return {"url": url, "error": str(error)}


if __name__ == "__main__":
    all_jobs = {}
    prior = {}
    if (OUT / "metadata.json").exists():
        all_jobs = {job["reference"]: job for job in json.loads((OUT / "metadata.json").read_text())}
    if (OUT / "details.json").exists():
        prior = {job["posting_id"]: job for job in json.loads((OUT / "details.json").read_text()) if "posting_id" in job}
    for word in sys.argv[1:] or ["intern", "attendant", "steward", "housekeeping"]:
        first = search(word)
        results = [first]
        count = first["totalJob"]
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results.extend(pool.map(lambda page: search(word, page), range(2, math.ceil(count / 10) + 1)))
        for result in results:
            for job in result["jobs"]:
                all_jobs[job["reference"]] = job
        (OUT / (word + "-search.json")).write_text(json.dumps(results, indent=2))
        print(json.dumps({"query": word, "reported": count, "downloaded": sum(len(x["jobs"]) for x in results), "unique_union": len(all_jobs)}), flush=True)
    (OUT / "metadata.json").write_text(json.dumps(list(all_jobs.values()), indent=2))
    results = list(prior.values())
    for row in results:
        evidence = NO_EXPERIENCE.search(row.get("description", ""))
        fields = {x["cfKey"]: x.get("value") for x in row.get("metadata", {}).get("customFields", [])}
        row["eligibility_evidence"] = evidence.group(0) if evidence else ""
        row["source_fields_eligible"] = evidence is not None and row.get("country") == "IN" and (not row.get("valid_through") or row["valid_through"] > datetime.datetime.now(datetime.timezone.utc).isoformat()) and fields.get("cf_external_posting_status") == "Posted"
    pending = [job for key, job in all_jobs.items() if key not in prior]
    if "" in sys.argv[1:]:
        pending = [job for job in pending if re.search(r"associate|attendant|expert|agent|intern|trainee|steward|reception|clerk|bell|valet|host|server|waiter|waitress|laundry|apprentice", job["title"], re.I) and not re.search(r"manager|director|supervisor|chef|head|senior", job["title"], re.I)]
    print(json.dumps({"new_details_to_check": len(pending)}), flush=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        for result in pool.map(detail, pending):
            results.append(result)
            if len(results) % 25 == 0:
                print(json.dumps({"checked": len(results), "eligible": sum(bool(x.get("source_fields_eligible")) for x in results), "errors": sum("error" in x for x in results)}), flush=True)
                (OUT / "details.json").write_text(json.dumps(results, indent=2))
    (OUT / "details.json").write_text(json.dumps(results, indent=2))
    eligible = [x for x in results if x.get("source_fields_eligible")]
    targets = [{"url": x["url"], "posting_id": x["posting_id"], "requisition_id": x["requisition_id"], "valid_through": x["valid_through"]} for x in eligible]
    inventory = {"checked_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "scope": "Explicit employer public URLs, refreshed individually. India internship and hotel operations searches; not the whole employer board.", "targets": targets}
    (OUT / "candidate-inventory.json").write_text(json.dumps(inventory, indent=2) + "\n")
    print(json.dumps({"candidate_urls": len(targets), "output": str(OUT / "candidate-inventory.json"), "errors": sum("error" in x for x in results)}), flush=True)
