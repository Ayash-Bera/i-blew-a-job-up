#!/usr/bin/env python3
"""Build an explicit public URL inventory from fresher search pages."""
import concurrent.futures
import datetime
import html
import json
from pathlib import Path
import re
import threading
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
OUT = Path("/tmp/firstrole-apna-collection")
OUT.mkdir(exist_ok=True)
FLIGHT = re.compile(r"self\.__next_f\.push\((\[1,.*?\])\)</script>", re.S)
STOP = threading.Event()


def fetch(url):
    for attempt in range(3):
        if STOP.is_set():
            raise RuntimeError("Stopped after access restriction")
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "IBlewAJobUp/1.0 public job listings"})
            with urllib.request.urlopen(request, timeout=35) as response:
                return response.read().decode()
        except urllib.error.HTTPError as error:
            if error.code in (403, 429, 481):
                STOP.set()
                raise
            if error.code not in (500, 502, 503, 504) or attempt == 2:
                raise
            time.sleep(attempt + 1)


def flight(text):
    return "".join(json.loads(match)[1] for match in FLIGHT.findall(text))


def get_object(text, key):
    marker = json.dumps(key) + ":"
    start = text.find(marker)
    if start < 0:
        raise ValueError("Missing public field " + key)
    return json.JSONDecoder().raw_decode(text[start + len(marker):])[0]


def page(index):
    url = "https://apna.co/jobs/freshers-jobs?page=" + str(index)
    data = get_object(flight(fetch(url)), "initialSSRJobFeedData")
    jobs = [row["data"] for row in data["jobsList"] if row.get("data", {}).get("jobPublicURL")]
    return {"page": index, "url": url, "jobs": jobs}


def detail(url):
    try:
        raw = get_object(flight(fetch(url)), "jobDetails")
        text = html.unescape(re.sub(r"<[^>]+>", " ", raw.get("rich_description", raw.get("description", ""))))
        international = raw.get("international_job_info", {}).get("is_international")
        expiry = raw.get("expiry", "")
        qualified = raw.get("status") == 0 and raw.get("min_experience") == 0 and "freshers can apply" in raw.get("experience_in_years", "").lower() and international is False and expiry >= datetime.date.today().isoformat()
        return {
            "posting_id": str(raw["id"]), "url": url, "title": raw["title"],
            "company": raw.get("organization", {}).get("name"),
            "location": raw.get("address", {}).get("city", {}).get("name"),
            "description": text, "eligibility_evidence": raw.get("experience_in_years"),
            "min_experience": raw.get("min_experience"), "international": international,
            "valid_through": expiry, "status": raw.get("status"), "qualified_by_source_fields": qualified,
            "checked_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        }
    except Exception as error:
        return {"url": url, "error": str(error)}


if __name__ == "__main__":
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        pages = list(pool.map(page, range(1, 19)))
    (OUT / "pages.json").write_text(json.dumps(pages, indent=2))
    urls = sorted({"https://apna.co" + job["jobPublicURL"] for result in pages for job in result["jobs"]})
    print(json.dumps({"pages": len(pages), "unique_urls": len(urls)}), flush=True)
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        for result in pool.map(detail, urls):
            results.append(result)
            if len(results) % 25 == 0:
                print(json.dumps({"checked": len(results), "source_field_eligible": sum(bool(x.get("qualified_by_source_fields")) for x in results), "errors": sum("error" in x for x in results)}), flush=True)
    (OUT / "details.json").write_text(json.dumps(results, indent=2))
    targets = [{"url": row["url"], "posting_id": row["posting_id"]} for row in results if row.get("qualified_by_source_fields")]
    inventory = {"checked_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "scope": "Discovery candidates require runtime schema validation and eligibility review before activation.", "targets": targets}
    (OUT / "candidate-inventory.json").write_text(json.dumps(inventory, indent=2) + "\n")
    print(json.dumps({"candidate_urls": len(targets), "output": str(OUT / "candidate-inventory.json"), "errors": sum("error" in x for x in results)}), flush=True)
