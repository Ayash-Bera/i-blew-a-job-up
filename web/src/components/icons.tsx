import type { CSSProperties } from "react";

export function Icon({ name, size = 18, className, style }: { name: "arrow" | "search" | "pin" | "briefcase" | "chevron" | "download" | "close" | "filter" | "clock" | "check" | "refresh" | "external"; size?: number; className?: string; style?: CSSProperties }) {
  const shapes = {
    arrow: <><path d="M5 12h14M13 6l6 6-6 6" /></>,
    search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4 4" /></>,
    pin: <><path d="M19 10c0 5-7 11-7 11S5 15 5 10a7 7 0 1 1 14 0Z" /><circle cx="12" cy="10" r="2" /></>,
    briefcase: <><rect x="3" y="7" width="18" height="14" rx="2" /><path d="M8 7V3h8v4M3 12c6 3 12 3 18 0M12 11v5" /></>,
    chevron: <path d="m8 10 4 4 4-4" />,
    download: <><path d="M12 3v12m-5-5 5 5 5-5M4 15v5h16v-5" /></>,
    close: <path d="m6 6 12 12M6 18 18 6" />,
    filter: <><path d="M4 7h16M4 17h16" /><circle cx="9" cy="7" r="2" /><circle cx="15" cy="17" r="2" /></>,
    clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
    check: <path d="m5 12 4 4L19 6" />,
    refresh: <><path d="M20 7v5h-5M4 17v-5h5" /><path d="M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1" /></>,
    external: <><path d="M14 3h7v7M21 3l-11 11M10 3H4v17h17v-6" /></>,
  };
  return <svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" className={className} style={style}>{shapes[name]}</svg>;
}
