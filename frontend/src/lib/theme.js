/**
 * Theme management — DOM manipulation for theme/dark mode.
 */
import { getTheme } from './api.js';

/** Resolve whether dark mode is active, considering 'auto' mode. */
function resolveDark(darkMode) {
  if (darkMode === 'auto') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  }
  return darkMode === true || darkMode === 'true';
}

/** Apply theme + dark mode to the document root. */
export function applyTheme(theme, darkMode) {
  const isDark = resolveDark(darkMode);
  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');
  if (theme.accent_color) {
    const root = document.documentElement;
    root.style.setProperty('--accent', theme.accent_color);
    const hex = theme.accent_color.replace('#', '');
    const r = parseInt(hex.substring(0, 2), 16);
    const g = parseInt(hex.substring(2, 4), 16);
    const b = parseInt(hex.substring(4, 6), 16);
    const alpha = isDark ? 0.15 : 0.08;
    root.style.setProperty('--accent-light', `rgba(${r},${g},${b},${alpha})`);
    // Derive hover (darken 15%) and text (white or black based on luminance)
    const hoverR = Math.max(0, Math.round(r * 0.85));
    const hoverG = Math.max(0, Math.round(g * 0.85));
    const hoverB = Math.max(0, Math.round(b * 0.85));
    root.style.setProperty('--accent-hover', `rgb(${hoverR},${hoverG},${hoverB})`);
    const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    root.style.setProperty('--accent-text', luminance > 0.5 ? '#0f172a' : '#ffffff');
  }
}

// Versioned localStorage key — bumped 2026-05-01 to invalidate the legacy
// "darkMode" entries that were written before Seeseo switched to dark default.
// Users keep their explicit toggles going forward, but the first load after
// the upgrade respects the server-side mode.
const DARK_KEY = 'darkMode_v2';
const LEGACY_DARK_KEY = 'darkMode';

/** Load theme from API + dark mode preference from localStorage.
 *  When the user has never set a preference (no v2 entry), we follow the
 *  server-side `theme.mode` config; if the server says "dark" or doesn't
 *  pick a side, we default to dark (Seeseo branding — Régis 2026-05-01).
 */
export async function loadThemeFromServer() {
  const t = await getTheme();
  // Drop the legacy key so the user gets a fresh start with the new defaults.
  if (localStorage.getItem(LEGACY_DARK_KEY) !== null) {
    localStorage.removeItem(LEGACY_DARK_KEY);
  }
  const saved = localStorage.getItem(DARK_KEY);
  let dark;
  if (saved === 'auto') {
    dark = 'auto';
  } else if (saved !== null) {
    dark = saved === 'true';
  } else if (t.mode === 'light') {
    dark = false;
  } else if (t.mode === 'auto') {
    dark = 'auto';
  } else {
    // server says "dark" (or anything else) → dark
    dark = true;
  }
  return { theme: t, darkMode: dark };
}

/** Persist dark mode preference to localStorage. */
export function saveDarkMode(darkMode) {
  if (darkMode === 'auto') {
    localStorage.setItem(DARK_KEY, 'auto');
  } else {
    localStorage.setItem(DARK_KEY, darkMode ? 'true' : 'false');
  }
}

/** Listen for OS color scheme changes and re-apply theme when in auto mode. */
export function listenColorScheme(getThemeState) {
  const mql = window.matchMedia('(prefers-color-scheme: dark)');
  mql.addEventListener('change', () => {
    const { theme, darkMode } = getThemeState();
    if (darkMode === 'auto') {
      applyTheme(theme, 'auto');
    }
  });
}
