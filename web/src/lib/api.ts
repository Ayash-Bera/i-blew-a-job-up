import type { Filters, JobsResponse, Run, Source, Stats } from "./types";
import { filterQuery } from "./query";

async function request<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/${path}`, { signal, cache: "no-store" });
  if (!response.ok) {
    throw new Error(response.status === 503
      ? "The job service is not connected yet. Please check back shortly."
      : "We could not reach the job service. Please try again.");
  }
  const data = await response.json();
  if (!data || typeof data !== "object") throw new Error("The job service returned an incomplete response. Please try again.");
  return data as T;
}

export const api = {
  jobs: async (filters: Filters, signal?: AbortSignal) => {
    const data = await request<JobsResponse>(`jobs?${filterQuery(filters)}`, signal);
    return {
      ...data,
      jobs: data.jobs ?? [],
      facets: { roles: data.facets?.roles ?? [], cities: data.facets?.cities ?? [], types: data.facets?.types ?? [], modes: data.facets?.modes ?? [] },
    };
  },
  stats: (signal?: AbortSignal) => request<Stats>("stats", signal),
  sources: async (signal?: AbortSignal) => {
    const data = await request<{ sources: Source[] }>("sources", signal);
    return { sources: data.sources ?? [] };
  },
  runs: async (signal?: AbortSignal) => {
    const data = await request<{ runs: Run[] }>("runs", signal);
    return { runs: data.runs ?? [] };
  },
  exportURL: "/api/export.csv",
};
