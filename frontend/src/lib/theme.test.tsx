import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  THEME_KEY,
  applyThemeClass,
  bootTheme,
  nextTheme,
  readStoredTheme,
  resolveDark,
  writeStoredTheme,
} from '@/lib/theme'

function fakeEl() {
  const classes = new Set<string>()
  return {
    classList: {
      toggle(token: string, force?: boolean) {
        if (force === true) classes.add(token)
        else if (force === false) classes.delete(token)
        else if (classes.has(token)) classes.delete(token)
        else classes.add(token)
      },
      contains(token: string) {
        return classes.has(token)
      },
    },
    _classes: classes,
  }
}

describe('theme', () => {
  let store: Map<string, string>
  let storage: Storage

  beforeEach(() => {
    store = new Map()
    storage = {
      get length() {
        return store.size
      },
      clear: () => store.clear(),
      getItem: (k) => (store.has(k) ? store.get(k)! : null),
      key: (i) => Array.from(store.keys())[i] ?? null,
      removeItem: (k) => {
        store.delete(k)
      },
      setItem: (k, v) => {
        store.set(k, String(v))
      },
    }
  })

  it('persists preference', () => {
    writeStoredTheme('dark', storage)
    expect(storage.getItem(THEME_KEY)).toBe('dark')
    expect(readStoredTheme(storage)).toBe('dark')
  })

  it('resolveDark: explicit overrides system', () => {
    expect(resolveDark('light', true)).toBe(false)
    expect(resolveDark('dark', false)).toBe(true)
    expect(resolveDark('system', true)).toBe(true)
    expect(resolveDark('system', false)).toBe(false)
  })

  it('bootTheme applies class from storage', () => {
    const el = fakeEl()
    writeStoredTheme('dark', storage)
    expect(bootTheme(el, { storage, systemDark: false })).toBe('dark')
    expect(el.classList.contains('dark')).toBe(true)
  })

  it('bootTheme system follows media', () => {
    const el = fakeEl()
    writeStoredTheme('system', storage)
    bootTheme(el, { storage, systemDark: true })
    expect(el.classList.contains('dark')).toBe(true)
    bootTheme(el, { storage, systemDark: false })
    expect(el.classList.contains('dark')).toBe(false)
  })

  it('cycles light → dark → system', () => {
    expect(nextTheme('light')).toBe('dark')
    expect(nextTheme('dark')).toBe('system')
    expect(nextTheme('system')).toBe('light')
  })

  it('applyThemeClass toggles correctly', () => {
    const el = fakeEl()
    applyThemeClass(el, 'light', true)
    expect(el.classList.contains('dark')).toBe(false)
  })
})

describe('ThemeProvider system listener', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    document.documentElement.classList.remove('dark')
    localStorage.removeItem(THEME_KEY)
  })

  it('system mode follows mocked matchMedia change', async () => {
    const { render, act } = await import('@testing-library/react')
    const { ThemeProvider, useTheme } = await import('@/components/ThemeProvider')
    const listeners = new Set<(e: MediaQueryListEvent) => void>()
    let matches = false
    const mql = {
      matches,
      media: '(prefers-color-scheme: dark)',
      addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => {
        listeners.add(fn)
      },
      removeEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => {
        listeners.delete(fn)
      },
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => true,
      onchange: null,
    }
    Object.defineProperty(mql, 'matches', {
      get: () => matches,
    })
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation(() => mql),
    )
    localStorage.setItem(THEME_KEY, 'system')

    function Probe() {
      const { preference } = useTheme()
      return <span data-testid="pref">{preference}</span>
    }

    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    )
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    await act(async () => {
      matches = true
      listeners.forEach((fn) => fn({ matches: true } as MediaQueryListEvent))
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('toggle persists and re-applies on remount', async () => {
    const { render, screen } = await import('@testing-library/react')
    const userEvent = (await import('@testing-library/user-event')).default
    const { ThemeProvider } = await import('@/components/ThemeProvider')
    const { ThemeToggle } = await import('@/components/ThemeToggle')
    const { I18nProvider } = await import('@/components/I18nProvider')
    localStorage.removeItem(THEME_KEY)

    const user = userEvent.setup()
    const { unmount } = render(
      <ThemeProvider initialPreference="light">
        <I18nProvider initialLang="en">
          <ThemeToggle />
        </I18nProvider>
      </ThemeProvider>,
    )
    expect(screen.getByTestId('theme-toggle')).toHaveAttribute('data-theme', 'light')
    await user.click(screen.getByTestId('theme-toggle'))
    expect(localStorage.getItem(THEME_KEY)).toBe('dark')
    unmount()

    render(
      <ThemeProvider>
        <I18nProvider initialLang="en">
          <ThemeToggle />
        </I18nProvider>
      </ThemeProvider>,
    )
    expect(screen.getByTestId('theme-toggle')).toHaveAttribute('data-theme', 'dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })
})
