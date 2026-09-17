import { ReactElement, ReactNode } from 'react'
import { render, RenderOptions } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '@/components/AuthProvider'
import { I18nProvider } from '@/components/I18nProvider'
import { ThemeProvider } from '@/components/ThemeProvider'
import { ToastProvider } from '@/components/ui/sonner'
import type { Lang } from '@/lib/i18n'
import type { ThemePreference } from '@/lib/theme'

/** Theme + i18n + toast. Pair with MemoryRouter + AuthProvider when the tree needs them. */
export function TestProviders({
  children,
  lang = 'en',
  theme = 'light',
}: {
  children: ReactNode
  lang?: Lang
  theme?: ThemePreference
}) {
  return (
    <ThemeProvider initialPreference={theme}>
      <I18nProvider initialLang={lang}>
        <ToastProvider>{children}</ToastProvider>
      </I18nProvider>
    </ThemeProvider>
  )
}

/** Full console chrome: router + auth. */
export function renderWithProviders(
  ui: ReactElement,
  opts: {
    lang?: Lang
    theme?: ThemePreference
    route?: string
    path?: string
  } & Omit<RenderOptions, 'wrapper'> = {},
) {
  const { lang = 'en', theme = 'light', route = '/', ...rest } = opts
  return render(ui, {
    wrapper: ({ children }) => (
      <TestProviders lang={lang} theme={theme}>
        <MemoryRouter initialEntries={[route]}>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      </TestProviders>
    ),
    ...rest,
  })
}

export { MemoryRouter, AuthProvider }
