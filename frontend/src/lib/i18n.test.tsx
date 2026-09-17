import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AppShell } from '@/components/AppShell'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import { resolveLang, writeStoredLang, LANG_KEY } from '@/lib/i18n'
import { translate } from '@/lib/i18n'
import de from '@/locales/de.json'
import en from '@/locales/en.json'
import type { Dict } from '@/lib/i18n'

describe('resolveLang', () => {
  it('defaults de-AT first visit to de', () => {
    expect(resolveLang(null, 'de-AT')).toBe('de')
    expect(resolveLang(null, 'de-DE')).toBe('de')
    expect(resolveLang(null, 'en-US')).toBe('en')
    expect(resolveLang(null, 'fr-FR')).toBe('en')
  })

  it('stored choice wins', () => {
    expect(resolveLang('en', 'de-DE')).toBe('en')
    expect(resolveLang('de', 'en-US')).toBe('de')
  })
})

describe('translate plurals', () => {
  it('uses German one/other forms', () => {
    const dict = de as Dict
    expect(translate(dict, 'inbox.title', { count: 1, showing: 1 })).toContain('Rechnung')
    expect(translate(dict, 'inbox.title', { count: 2, showing: 2 })).toContain('Rechnungen')
  })
})

describe('i18n UI chrome', () => {
  it('nav + titles render German when de active', async () => {
    vi.spyOn(await import('@/lib/api'), 'health').mockResolvedValue({
      status: 'ok',
      version: '0.5.0',
    })
    render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/']}>
          <AuthProvider>
            <Routes>
              <Route element={<AppShell />}>
                <Route index element={<h1>home</h1>} />
              </Route>
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByTitle('Posteingang')).toBeInTheDocument()
    expect(screen.getByTitle('Neue Rechnung')).toBeInTheDocument()
    expect(screen.getByTitle('Einstellungen')).toBeInTheDocument()
  })

  it('nav renders English when en active', async () => {
    vi.spyOn(await import('@/lib/api'), 'health').mockResolvedValue({
      status: 'ok',
      version: '0.5.0',
    })
    render(
      <TestProviders lang="en">
        <MemoryRouter basename="/app" initialEntries={['/app/']}>
          <AuthProvider>
            <Routes>
              <Route element={<AppShell />}>
                <Route index element={<h1>home</h1>} />
              </Route>
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByTitle('Inbox')).toBeInTheDocument()
    expect(screen.getByTitle('New invoice')).toBeInTheDocument()
  })

  it('language switcher persists across remount', async () => {
    localStorage.removeItem(LANG_KEY)
    writeStoredLang('en')
    expect(localStorage.getItem(LANG_KEY)).toBe('en')
    expect(resolveLang(localStorage.getItem(LANG_KEY), 'de-DE')).toBe('en')
  })
  it('lang switcher toggles DE|EN', async () => {
    vi.spyOn(await import('@/lib/api'), 'health').mockResolvedValue({
      status: 'ok',
      version: '0.5.0',
    })
    const user = userEvent.setup()
    render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/']}>
          <AuthProvider>
            <Routes>
              <Route element={<AppShell />}>
                <Route index element={<h1>home</h1>} />
              </Route>
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByTestId('lang-switcher')).toHaveAttribute('data-lang', 'de')
    await user.click(screen.getByRole('button', { name: 'EN' }))
    expect(screen.getByTestId('lang-switcher')).toHaveAttribute('data-lang', 'en')
    expect(localStorage.getItem(LANG_KEY)).toBe('en')
    expect(screen.getByTitle('Inbox')).toBeInTheDocument()
  })
})

// silence unused en import in case tree-shaking complains
void en
