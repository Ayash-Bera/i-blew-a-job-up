"use client";

import { useEffect, useSyncExternalStore } from "react";
import { THEME_STORAGE_KEY, type ThemePreference } from "@/lib/theme";

const THEME_EVENT = "theme-preference-change";
let sessionPreference: ThemePreference | undefined;

function getPreference(): ThemePreference {
  if (sessionPreference) return sessionPreference;
  try {
    const saved = localStorage.getItem(THEME_STORAGE_KEY);
    if (saved === "light" || saved === "dark") return saved;
  } catch {}
  return "system";
}

function subscribe(onChange: () => void) {
  function onStorage(event: StorageEvent) {
    if (event.key === THEME_STORAGE_KEY || event.key === null) {
      sessionPreference = undefined;
      onChange();
    }
  }
  window.addEventListener(THEME_EVENT, onChange);
  window.addEventListener("storage", onStorage);
  return () => {
    window.removeEventListener(THEME_EVENT, onChange);
    window.removeEventListener("storage", onStorage);
  };
}

function setPreference(value: ThemePreference) {
  // Keep the control usable when browser storage is unavailable.
  sessionPreference = value;
  try {
    localStorage.setItem(THEME_STORAGE_KEY, value);
  } catch {}
  window.dispatchEvent(new Event(THEME_EVENT));
}

export function ThemeControl() {
  const preference = useSyncExternalStore(subscribe, getPreference, () => "system");

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      const current = getPreference();
      document.documentElement.dataset.theme = current === "system"
        ? media.matches ? "dark" : "light"
        : current;
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [preference]);

  return <div className="theme-control">
    <label htmlFor="color-theme" className="sr-only">Color theme</label>
    <select id="color-theme" className="theme-select" value={preference}
      onChange={(event) => setPreference(event.target.value as ThemePreference)}>
      <option value="system">System</option>
      <option value="light">Light</option>
      <option value="dark">Dark</option>
    </select>
  </div>;
}
