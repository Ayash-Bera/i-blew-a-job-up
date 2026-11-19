export type ThemePreference = "system" | "light" | "dark";

export const THEME_STORAGE_KEY = "i-blew-a-job-up-theme";

export const themeBootstrapScript = `(() => {
  let preference = "system";
  try {
    const saved = localStorage.getItem(${JSON.stringify(THEME_STORAGE_KEY)});
    if (saved === "light" || saved === "dark") preference = saved;
  } catch {}
  const dark = preference === "dark" || (preference === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
})();`;
