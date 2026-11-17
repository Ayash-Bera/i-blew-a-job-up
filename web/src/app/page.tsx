import { JobBoard } from "@/components/job-board";
import type { Filters } from "@/lib/types";

export default async function Home({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const params = await searchParams;
  const value = (key: string) => typeof params[key] === "string" ? params[key] as string : "";
  const page = Number(value("page"));
  const filters: Filters = { q: value("q"), role: value("role"), city: value("city"), type: value("type"), mode: value("mode"), page: Number.isSafeInteger(page) && page > 0 ? page : 1 };
  return <JobBoard initialFilters={filters} />;
}
