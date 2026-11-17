export const emptyFilters = { q: "", role: "", city: "", type: "", mode: "", page: 1 };

export function filterQuery(filters: { q: string; role: string; city: string; type: string; mode: string; page: number }, includeLimit = true) {
  const query = new URLSearchParams();
  for (const key of ["q", "role", "city", "type", "mode"] as const) {
    const value = filters[key].trim();
    if (value) query.set(key, value);
  }
  if (filters.page > 1) query.set("page", String(filters.page));
  if (includeLimit) query.set("limit", "20");
  return query.toString();
}

export function safeURL(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  try {
    const url = new URL(value);
    if ((url.protocol === "https:" || url.protocol === "http:") && !url.username && !url.password) return url.href;
  } catch { /* Invalid source links cannot become navigation targets. */ }
  return undefined;
}

export function isStale(value: string | null, now = Date.now()) {
  if (!value) return true;
  const time = Date.parse(value);
  return !Number.isFinite(time) || now - time > 30 * 60 * 60 * 1000;
}

export function formatDate(value: string | null | undefined, includeTime = false) {
  if (!value || !Number.isFinite(Date.parse(value))) return "Not recorded";
  return new Intl.DateTimeFormat("en-IN", {
    timeZone: "Asia/Kolkata",
    day: "numeric",
    month: "short",
    year: "numeric",
    ...(includeTime ? { hour: "2-digit", minute: "2-digit", hour12: true } : {}),
  }).format(new Date(value)) + (includeTime ? " IST" : "");
}

export function label(value: string) {
  if (!value || value === "unspecified") return "Not specified";
  return value.charAt(0).toUpperCase() + value.slice(1).replaceAll("_", " ");
}
