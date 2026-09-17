import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SecurityPage } from '@/pages/SecurityPage'
import { UsersPage } from '@/pages/UsersPage'
import { AppShell } from '@/components/AppShell'
import { renderWithProviders } from '@/test/render'
import * as api from '@/lib/api'

function json(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}

describe('Security + Users + role UX', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('2FA card: setup QR → confirm recovery → remove', async () => {
    let enrolled = false
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'users' })
        if (url.includes('/me')) {
          return json({
            user: {
              id: '1',
              email: 'a@b.c',
              name: 'A',
              role: 'admin',
              totp_enabled: enrolled,
              totp_confirmed_at: enrolled ? '2026-09-11T12:00:00Z' : undefined,
            },
          })
        }
        if (url.includes('/2fa/setup')) {
          return json({
            secret: 'SECRET',
            provisioning_uri: 'otpauth://totp/Factura:a@b.c?secret=SECRET',
            qr: 'data:image/png;base64,aaa',
          })
        }
        if (url.includes('/2fa/confirm')) {
          enrolled = true
          return json({
            recovery_codes: Array.from({ length: 10 }, (_, i) => `aaaa-bbb${i}`),
          })
        }
        if (url.includes('/2fa') && init?.method === 'DELETE') {
          enrolled = false
          return new Response(null, { status: 204 })
        }
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev', auth: 'users' })
        return json({})
      }),
    )
    localStorage.setItem('factura_session', 'sess')
    renderWithProviders(<SecurityPage />, { route: '/settings/security' })
    await waitFor(() => screen.getByTestId('2fa-setup'))
    await userEvent.click(screen.getByTestId('2fa-setup'))
    await waitFor(() => screen.getByTestId('2fa-qr-dialog'))
    expect(screen.getByAltText('QR').getAttribute('src')).toMatch(/^data:image\/png/)
    await userEvent.type(screen.getByTestId('2fa-confirm-code'), '123456')
    await userEvent.click(screen.getByRole('button', { name: /confirm/i }))
    await waitFor(() => screen.getByTestId('recovery-block'))
    await waitFor(() => screen.getByTestId('2fa-remove'))
    await userEvent.click(screen.getByTestId('2fa-remove'))
    // Confirm dialog — the destructive action inside the dialog.
    const dialogs = screen.getAllByRole('button', { name: 'Remove 2FA' })
    await userEvent.click(dialogs[dialogs.length - 1])
    await waitFor(() => screen.getByTestId('2fa-setup'))
  })

  it('Users page hidden for editor; admin sees table', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'users' })
        if (url.includes('/me')) {
          return json({
            user: { id: '1', email: 'e@t.de', name: 'Ed', role: 'editor', totp_enabled: false },
          })
        }
        if (url.includes('/users')) {
          return json({ error: 'forbidden' }, 401)
        }
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev' })
        return json({})
      }),
    )
    localStorage.setItem('factura_session', 'sess')
    renderWithProviders(<UsersPage />, { route: '/settings/users' })
    await waitFor(() => screen.getByTestId('users-forbidden'))
  })

  it('Users page admin: table + add dialog', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'users' })
        if (url.includes('/me')) {
          return json({
            user: { id: '1', email: 'a@t.de', name: 'Ad', role: 'admin', totp_enabled: false },
          })
        }
        if (url.includes('/users') && !url.includes('PATCH')) {
          return json({
            users: [
              {
                id: '1',
                email: 'a@t.de',
                name: 'Ad',
                role: 'admin',
                totp_enabled: false,
                created_at: '2026-09-11T00:00:00Z',
              },
            ],
          })
        }
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev' })
        return json({})
      }),
    )
    localStorage.setItem('factura_session', 'sess')
    renderWithProviders(<UsersPage />, { route: '/settings/users' })
    await waitFor(() => screen.getByTestId('users-table'))
    await userEvent.click(screen.getByTestId('users-add'))
    await waitFor(() => screen.getByTestId('users-add-dialog'))
    expect(screen.getByTestId('users-role-select')).toBeInTheDocument()
  })

  it('viewer: New-invoice nav absent; forbidden toast does not crash', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'users' })
        if (url.includes('/me')) {
          return json({
            user: { id: '1', email: 'v@t.de', name: 'View', role: 'viewer', totp_enabled: false },
          })
        }
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev', auth: 'users' })
        return json({})
      }),
    )
    localStorage.setItem('factura_session', 'sess')
    renderWithProviders(
      <AppShell />,
      { route: '/' },
    )
    await waitFor(() => screen.getByTestId('user-header'))
    expect(screen.getByTestId('role-chip')).toHaveTextContent('viewer')
    expect(screen.queryByText('New invoice')).not.toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()

    // Hand-rolled forbidden — surfaces as ApiError, no crash.
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes('/invoices') && !url.includes('/auth')) {
        return json({ error: 'forbidden' }, 401)
      }
      if (url.includes('/auth-status')) return json({ mode: 'users' })
      if (url.includes('/me')) {
        return json({
          user: { id: '1', email: 'v@t.de', name: 'View', role: 'viewer', totp_enabled: false },
        })
      }
      return json({})
    })
    await expect(api.ingest(new Uint8Array([1, 2, 3]))).rejects.toMatchObject({
      status: 401,
      message: 'forbidden',
    })
  })

  it('open mode: app loads without login redirect; header has no user chip', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'open' })
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev', auth: 'open' })
        return json({})
      }),
    )
    renderWithProviders(<AppShell />, { route: '/' })
    await waitFor(() => screen.getByText('Factura'))
    expect(screen.queryByTestId('user-header')).not.toBeInTheDocument()
    expect(screen.getByText('Inbox')).toBeInTheDocument()
  })

  it('logout clears session and lands on login', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) return json({ mode: 'users' })
        if (url.includes('/me')) {
          return json({
            user: { id: '1', email: 'a@t.de', name: 'Ad', role: 'admin', totp_enabled: false },
          })
        }
        if (url.includes('/health')) return json({ status: 'ok', version: 'dev' })
        return json({})
      }),
    )
    localStorage.setItem('factura_session', 'sess')
    renderWithProviders(<AppShell />, { route: '/' })
    await waitFor(() => screen.getByTestId('logout'))
    await userEvent.click(screen.getByTestId('logout'))
    expect(localStorage.getItem('factura_session')).toBeNull()
  })
})
