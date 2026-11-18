"""Check a running service using its public endpoints."""

import argparse
import csv
import io
import json
import urllib.error
import urllib.parse
import urllib.request


def verify(base, minimum, snapshot=False):
    def get(path):
        request = urllib.request.Request(base.rstrip("/") + path, headers={"User-Agent": "IBlewAJobUp-verification/1.0"})
        with urllib.request.urlopen(request, timeout=30) as response:
            assert response.status == 200
            return response.read().decode()

    stats = json.loads(get("/api/stats"))
    if snapshot:
        assert stats["refresh_status"] == "snapshot_read_only"
        assert stats["tagged_jobs"] == 0
        assert stats["costs"]["total_usd"] is None
        assert stats["evaluation"]["reviewed"] == 0
    else:
        assert stats["refresh_status"] != "snapshot_read_only", "Read-only preview is not a completed deployment"
    first = json.loads(get("/api/jobs?limit=100&page=1"))
    jobs = list(first["jobs"])
    page = 2
    while len(jobs) < first["total"]:
        result = json.loads(get(f"/api/jobs?limit=100&page={page}"))
        assert result["jobs"], "Pagination ended before the reported total"
        assert result["total"] == first["total"], "Dataset changed during verification; retry on a frozen run"
        jobs.extend(result["jobs"])
        page += 1
        assert page < 1000, "Pagination did not converge"
    assert len(jobs) == len({job["id"] for job in jobs}), "Repeated canonical IDs"
    assert len(jobs) == first["total"] == stats["active_jobs"], "API counts disagree"
    assert len(jobs) >= minimum, f"Only {len(jobs)} published jobs, need {minimum}"
    for job in jobs:
        assert job["title"] and job["company"] and job["eligibility_evidence"]
        for field in ("source_url", "apply_url"):
            url = urllib.parse.urlsplit(job[field])
            assert url.scheme in {"http", "https"} and url.hostname and not url.username
        assert isinstance(job["cities"], list)
        if snapshot:
            assert job["tagging_method"] != "llm", "Snapshot verification expects explicitly pending tags"
        else:
            assert job["tagging_method"] == "llm", "Published job has no measured model tags"
        assert not any(key in job for key in ("api_key", "password", "model_key", "database_url"))
    if jobs:
        role = jobs[0]["role"]
        filtered = json.loads(get("/api/jobs?" + urllib.parse.urlencode({"role": role, "limit": 100})))
        assert filtered["total"] == sum(job["role"] == role for job in jobs)
        assert all(job["role"] == role for job in filtered["jobs"])
        if jobs[0]["cities"]:
            city = jobs[0]["cities"][0]
            filtered = json.loads(get("/api/jobs?" + urllib.parse.urlencode({"city": city, "limit": 100})))
            assert filtered["total"] == sum(city in job["cities"] for job in jobs)
    empty = json.loads(get("/api/jobs?q=firstrole-impossible-query-42da7e"))
    assert empty["total"] == 0 and empty["jobs"] == []
    exported = list(csv.DictReader(io.StringIO(get("/api/export.csv"))))
    assert len(exported) == len(jobs), "CSV and public list counts disagree"
    for path in ("/api/sources", "/api/runs"):
        json.loads(get(path))
    try:
        get("/api/cron")
        raise AssertionError("Unauthenticated refresh was accepted")
    except urllib.error.HTTPError as error:
        assert error.code in {401, 403}, f"Unexpected refresh denial status {error.code}"
    mode = "read-only preview, tags pending" if snapshot else "model-tagged production"
    print(f"API verification passed ({mode}): {len(jobs)} unique posts; filters, export, counts and refresh protection agree")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--base", required=True)
    parser.add_argument("--min-jobs", type=int, default=200)
    parser.add_argument("--snapshot", action="store_true", help="Verify an incomplete read-only preview separately from production acceptance")
    args = parser.parse_args()
    verify(args.base, args.min_jobs, args.snapshot)
