/** Hand-rolled DE/EN i18n — zero deps, UI chrome only.
 *  Validation `human` / engine payload strings are NEVER passed through `t()`. */

export const LANG_KEY = 'factura:lang'
export type Lang = 'de' | 'en'

export type Dict = Record<string, string | { one: string; other: string }>

export function isLang(v: unknown): v is Lang {
  return v === 'de' || v === 'en'
}

/** First visit: de-* → de, else en. Persisted choice wins. */
export function resolveLang(
  stored: string | null | undefined,
  navigatorLanguage: string | undefined,
): Lang {
  if (isLang(stored)) return stored
  const nav = (navigatorLanguage || '').toLowerCase()
  if (nav.startsWith('de')) return 'de'
  return 'en'
}

export function readStoredLang(
  storage: Pick<Storage, 'getItem'> | null | undefined = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): string | null {
  try {
    return storage?.getItem(LANG_KEY) ?? null
  } catch {
    return null
  }
}

export function writeStoredLang(
  lang: Lang,
  storage: Pick<Storage, 'setItem'> | null | undefined = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): void {
  try {
    storage?.setItem(LANG_KEY, lang)
  } catch {
    /* ignore */
  }
}

function interpolate(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template
  return template.replace(/\{\{(\w+)\}\}/g, (_, key: string) =>
    vars[key] != null ? String(vars[key]) : `{{${key}}}`,
  )
}

/**
 * Look up a key. Supports nested plural forms `{ one, other }` when `count`
 * is provided (German/English both use one/other).
 */
export function translate(
  dict: Dict,
  key: string,
  vars?: Record<string, string | number>,
  fallbackDict?: Dict,
): string {
  const raw = dict[key] ?? fallbackDict?.[key]
  if (raw == null) return key
  if (typeof raw === 'string') return interpolate(raw, vars)
  const count = typeof vars?.count === 'number' ? vars.count : Number(vars?.count)
  const form = count === 1 ? raw.one : raw.other
  return interpolate(form, vars)
}
