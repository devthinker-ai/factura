import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import {
  type ThemePreference,
  applyThemeClass,
  nextTheme,
  readStoredTheme,
  writeStoredTheme,
} from '@/lib/theme'

interface ThemeContextValue {
  preference: ThemePreference
  setPreference: (pref: ThemePreference) => void
  cycle: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function systemDarkQuery(): MediaQueryList | null {
  if (typeof window === 'undefined' || !window.matchMedia) return null
  return window.matchMedia('(prefers-color-scheme: dark)')
}

export function ThemeProvider({
  children,
  initialPreference,
}: {
  children: ReactNode
  initialPreference?: ThemePreference
}) {
  const [preference, setPreferenceState] = useState<ThemePreference>(
    () => initialPreference ?? readStoredTheme(),
  )

  const setPreference = useCallback((pref: ThemePreference) => {
    setPreferenceState(pref)
    writeStoredTheme(pref)
  }, [])

  const cycle = useCallback(() => {
    setPreferenceState((prev) => {
      const next = nextTheme(prev)
      writeStoredTheme(next)
      return next
    })
  }, [])

  useEffect(() => {
    const mq = systemDarkQuery()
    const apply = () => {
      applyThemeClass(
        document.documentElement,
        preference,
        mq?.matches ?? false,
      )
    }
    apply()
    if (preference !== 'system' || !mq) return
    const onChange = () => apply()
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [preference])

  const value = useMemo(
    () => ({ preference, setPreference, cycle }),
    [preference, setPreference, cycle],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme requires ThemeProvider')
  return ctx
}
