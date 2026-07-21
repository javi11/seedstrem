// Theme persistence. Dark-first, remembered in localStorage, applied to the
// <html data-theme> attribute (which daisyUI reads).

export type ThemeName = "seedstrem-dark" | "seedstrem-light";

const STORAGE_KEY = "seedstrem-theme";
const DEFAULT_THEME: ThemeName = "seedstrem-dark";

export function getStoredTheme(): ThemeName {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "seedstrem-dark" || v === "seedstrem-light") return v;
  } catch {
    /* localStorage unavailable (private mode / SSR) — fall through */
  }
  return DEFAULT_THEME;
}

export function applyTheme(theme: ThemeName): void {
  document.documentElement.setAttribute("data-theme", theme);
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    /* best-effort persistence */
  }
}

export function isDark(theme: ThemeName): boolean {
  return theme === "seedstrem-dark";
}

export function toggleTheme(current: ThemeName): ThemeName {
  return current === "seedstrem-dark" ? "seedstrem-light" : "seedstrem-dark";
}
