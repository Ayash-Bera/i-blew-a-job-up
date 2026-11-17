#!/usr/bin/env python3
"""Measure public employer search feeds; output research only, never publish."""
import concurrent.futures
import json
from pathlib import Path
import re
import time
import urllib.error
import urllib.request

BOARDS = [
    ("nvidia", "nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite"),
    ("intel", "intel.wd1.myworkdayjobs.com", "External"),
    ("micron", "micron.wd1.myworkdayjobs.com", "External"),
    ("qualcomm", "qualcomm.wd5.myworkdayjobs.com", "External"),
    ("gehc", "gehc.wd5.myworkdayjobs.com", "GEHC_ExternalSite"),
    ("medtronic", "medtronic.wd1.myworkdayjobs.com", "MedtronicCareers"),
    ("itron", "itron.wd5.myworkdayjobs.com", "Itron"),
    ("illumina", "illumina.wd1.myworkdayjobs.com", "illumina-careers"),
    ("ag", "ag.wd3.myworkdayjobs.com", "Airbus"),
    ("fortrea", "fortrea.wd1.myworkdayjobs.com", "Fortrea"),
    ("db", "db.wd3.myworkdayjobs.com", "DBWebsite"),
    ("morningstar", "morningstar.wd5.myworkdayjobs.com", "morningstar"),
    ("jll", "jll.wd1.myworkdayjobs.com", "jllcareers"),
    ("rockwellautomation", "rockwellautomation.wd1.myworkdayjobs.com", "External_Rockwell_Automation"),
    ("motorolasolutions", "motorolasolutions.wd5.myworkdayjobs.com", "Careers"),
    ("wex", "wex.wd5.myworkdayjobs.com", "WEXInc"),
    ("trimble", "trimble.wd1.myworkdayjobs.com", "TrimbleCareers"),
    ("sbd", "sbd.wd1.myworkdayjobs.com", "External"),
    ("philips", "philips.wd3.myworkdayjobs.com", "jobs-and-careers"),
    ("hpe", "hpe.wd5.myworkdayjobs.com", "Jobsathpe"),
    ("mastercard", "mastercard.wd1.myworkdayjobs.com", "CorporateCareers"),
    ("pwc", "pwc.wd3.myworkdayjobs.com", "Global_Experienced_Careers"),
    ("gsk", "gsk.wd5.myworkdayjobs.com", "GSKCareers"),
    ("thomsonreuters", "thomsonreuters.wd5.myworkdayjobs.com", "External_Career_Site"),
]
OUT = Path("/tmp/firstrole-workday-probe")
OUT.mkdir(exist_ok=True)


def get_json(url, body=None):
    request = urllib.request.Request(url, data=None if body is None else json.dumps(body).encode(), headers={"User-Agent": "FirstRole/1.0 public-jobs-research", "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=35) as response:
        return json.load(response)


def india_facet(facets):
    for facet in facets:
        for value in facet.get("values", []):
            if value.get("descriptor", "").lower() == "india":
                return {facet["facetParameter"]: [value["id"]]}
        nested = india_facet(facet.get("values", []))
        if nested:
            return nested
    return {}


def probe(board):
    tenant, host, site = board
    base = f"https://{host}/wday/cxs/{tenant}/{site}"
    result = {"tenant": tenant, "url": f"https://{host}/{site}", "queries": []}
    try:
        initial = get_json(base + "/jobs", {"appliedFacets": {}, "limit": 20, "offset": 0, "searchText": ""})
        facet = india_facet(initial.get("facets", []))
        result["india_facet"] = facet
        if not facet:
            result["error"] = "No explicit India facet"
            return result
        union = {}
        for query in ["intern", "apprentice", "graduate", "trainee", "fresher"]:
            records = []
            offset = 0
            total = None
            seen = set()
            while True:
                payload = get_json(base + "/jobs", {"appliedFacets": facet, "limit": 20, "offset": offset, "searchText": query})
                items = payload.get("jobPostings", [])
                if total is None:
                    total = payload["total"]
                for item in items:
                    key = item["externalPath"]
                    if key in seen:
                        raise RuntimeError("Repeated posting during pagination")
                    seen.add(key)
                records.extend(items)
                if len(records) >= total:
                    break
                if not items:
                    raise RuntimeError("Search ended before reported total")
                if offset > 3000:
                    raise RuntimeError("Safety bound reached")
                offset += len(items)
            result["queries"].append({"query": query, "total": total, "fetched": len(records)})
            for item in records:
                item["query"] = query
                union[item["externalPath"]] = item
        result["unique_search_results"] = len(union)
        candidates = [x for x in union.values() if re.search(r"\b(intern\b|internship|apprentice|graduate|trainee|fresher|entry.level)", x["title"], re.I)]
        result["title_candidates"] = len(candidates)
        result["candidates"] = candidates
        (OUT / f"{tenant}-search.json").write_text(json.dumps(result, indent=2))
        return result
    except Exception as error:
        result["error"] = str(error)
        return result


if __name__ == "__main__":
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
        results = list(pool.map(probe, BOARDS))
    (OUT / "summary.json").write_text(json.dumps(results, indent=2))
    for result in results:
        print(json.dumps({k: v for k, v in result.items() if k not in ("candidates", "india_facet")}), flush=True)
