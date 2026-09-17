import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import de from '@/locales/de.json'
import en from '@/locales/en.json'
import {
  type Dict,
  type Lang,
  readStoredLang,
  resolveLang,
  translate,
  writeStoredLang,
} from '@/lib/i18n'

const catalogs: Record<Lang, Dict> = {
  de: de as Dict,
  en: en as Dict,
}

type TFunc = (key: string, vars?: Record<string, string | number>) => string

interface I18nContextValue {
  lang: Lang
  setLang: (lang: Lang) => void
  t: TFunc
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function I18nProvider({
  children,
  initialLang,
}: {
  children: ReactNode
  /** Override for tests; otherwise resolved from storage + navigator. */
  initialLang?: Lang
}) {
  const [lang, setLangState] = useState<Lang>(() => {
    if (initialLang) return initialLang
    const nav =
      typeof navigator !== 'undefined' ? navigator.language : undefined
    return resolveLang(readStoredLang(), nav)
  })

  const setLang = useCallback((next: Lang) => {
    setLangState(next)
    writeStoredLang(next)
  }, [])

  useEffect(() => {
    document.documentElement.lang = lang === 'de' ? 'de' : 'en'
  }, [lang])

  const t = useCallback<TFunc>(
    (key, vars) => translate(catalogs[lang], key, vars, catalogs.en),
    [lang],
  )

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t])

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n requires I18nProvider')
  return ctx
}

export function useT(): TFunc {
  return useI18n().t
}
