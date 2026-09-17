/** Theme preference + DOM class helpers. Extracted so the no-flash inline
 *  script logic can be unit-tested without a browser. */

export const THEME_KEY = 'factura:theme'
export type ThemePreference = 'light' | 'dark' | 'system'

export function isThemePreference(v: unknown): v is ThemePreference {
  return v === 'light' || v === 'dark' || v === 'system'
}

export function readStoredTheme(
  storage: Pick<Storage, 'getItem'> | null | undefined = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): ThemePreference {
  try {
    const raw = storage?.getItem(THEME_KEY)
    if (isThemePreference(raw)) return raw
  } catch {
    /* ignore */
  }
  return 'system'
}

export function writeStoredTheme(
  pref: ThemePreference,
  storage: Pick<Storage, 'setItem'> | null | undefined = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): void {
  try {
    storage?.setItem(THEME_KEY, pref)
  } catch {
    /* ignore */
  }
}

/** Resolve preference against a media-query dark flag. */
export function resolveDark(
  pref: ThemePreference,
  systemDark: boolean,
): boolean {
  if (pref === 'dark') return true
  if (pref === 'light') return false
  return systemDark
}

/** Apply (or remove) the `dark` class on an element — typically `<html>`. */
export function applyThemeClass(
  el: { classList: { toggle: (token: string, force?: boolean) => void } },
  pref: ThemePreference,
  systemDark: boolean,
): boolean {
  const dark = resolveDark(pref, systemDark)
  el.classList.toggle('dark', dark)
  return dark
}

/** Cycle: light → dark → system → light. */
export function nextTheme(pref: ThemePreference): ThemePreference {
  if (pref === 'light') return 'dark'
  if (pref === 'dark') return 'system'
  return 'light'
}

/**
 * Boot-time apply used by the inline `<head>` script and by unit tests.
 * Keeps the no-flash path identical to runtime.
 */
export function bootTheme(
  el: { classList: { toggle: (token: string, force?: boolean) => void } },
  opts: {
    storage?: Pick<Storage, 'getItem'> | null
    systemDark?: boolean
  } = {},
): ThemePreference {
  const pref = readStoredTheme(opts.storage ?? null)
  const systemDark =
    opts.systemDark ??
    (typeof window !== 'undefined'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : false)
  applyThemeClass(el, pref, systemDark)
  return pref
}
