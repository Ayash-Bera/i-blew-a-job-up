import { NextRequest, NextResponse } from "next/server";

const allowed = new Set(["jobs", "stats", "sources", "runs", "export.csv"]);

export async function GET(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path } = await context.params;
  const endpoint = path.join("/");
  if (!allowed.has(endpoint)) return NextResponse.json({ error: "Not found" }, { status: 404 });

  const base = process.env.API_URL;
  if (!base) return NextResponse.json({ error: "Job service not configured" }, { status: 503 });

  try {
    const url = new URL(`/api/${endpoint}`, base);
    url.search = request.nextUrl.search;
    const upstream = await fetch(url, {
      headers: { Accept: endpoint === "export.csv" ? "text/csv" : "application/json" },
      cache: "no-store",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(25_000)]),
    });
    if (!upstream.ok) return NextResponse.json({ error: "Job service request failed" }, { status: upstream.status });

    const headers = new Headers({
      "Content-Type": endpoint === "export.csv" ? "text/csv; charset=utf-8" : "application/json",
      "Cache-Control": "no-store",
    });
    if (endpoint === "export.csv") headers.set("Content-Disposition", "attachment; filename=i-blew-a-job-up-jobs.csv");
    return new Response(upstream.body, { headers, status: upstream.status });
  } catch {
    return NextResponse.json({ error: "Job service temporarily unavailable" }, { status: 502 });
  }
}
