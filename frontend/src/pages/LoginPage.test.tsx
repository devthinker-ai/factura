import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LoginPage } from '@/pages/LoginPage'
import { renderWithProviders } from '@/test/render'

function mockFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString()
      return handler(url, init)
    }),
  )
}

describe('LoginPage', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders DE and EN chrome labels', async () => {
    mockFetch((url) => {
      if (url.includes('/auth-status')) {
        return new Response(JSON.stringify({ mode: 'users' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } })
    })

    renderWithProviders(<LoginPage />, { lang: 'en', route: '/login' })
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument())
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('renders German chrome', async () => {
    mockFetch((url) => {
      if (url.includes('/auth-status')) {
        return new Response(JSON.stringify({ mode: 'users' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200 })
    })
    renderWithProviders(<LoginPage />, { lang: 'de', route: '/login' })
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Anmelden' })).toBeInTheDocument(),
    )
    expect(screen.getByLabelText('E-Mail')).toBeInTheDocument()
    expect(screen.getByLabelText('Passwort')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Anmelden' })).toBeInTheDocument()
  })

  it('shows API 401 body verbatim in DE mode', async () => {
    mockFetch((url, init) => {
      if (url.includes('/auth-status')) {
        return new Response(JSON.stringify({ mode: 'users' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/login') && init?.method === 'POST') {
        return new Response(JSON.stringify({ error: 'invalid credentials' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200 })
    })
    renderWithProviders(<LoginPage />, { lang: 'de', route: '/login' })
    await waitFor(() => screen.getByLabelText('E-Mail'))
    await userEvent.type(screen.getByLabelText('E-Mail'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Passwort'), 'password12')
    await userEvent.click(screen.getByRole('button', { name: 'Anmelden' }))
    await waitFor(() => expect(screen.getByTestId('login-error')).toHaveTextContent('invalid credentials'))
  })

  it('MFA step: 6-digit input, recovery toggle, 429 countdown', async () => {
    let loginCalls = 0
    mockFetch((url, init) => {
      if (url.includes('/auth-status')) {
        return new Response(JSON.stringify({ mode: 'users' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/login') && init?.method === 'POST') {
        loginCalls++
        return new Response(
          JSON.stringify({ mfa_required: true, mfa_token: 'mfa.jwt' }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        )
      }
      if (url.includes('/login/mfa')) {
        return new Response(JSON.stringify({ error: 'too many attempts' }), {
          status: 429,
          headers: { 'Content-Type': 'application/json', 'Retry-After': '3' },
        })
      }
      return new Response('{}', { status: 200 })
    })
    renderWithProviders(<LoginPage />, { lang: 'en', route: '/login' })
    await waitFor(() => screen.getByLabelText('Email'))
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Password'), 'password12')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    await waitFor(() => expect(screen.getByTestId('mfa-code')).toBeInTheDocument())
    expect(loginCalls).toBe(1)

    await userEvent.click(screen.getByText('Use a recovery code'))
    expect(screen.getByTestId('mfa-recovery')).toBeInTheDocument()
    await userEvent.type(screen.getByTestId('mfa-recovery'), 'abcd-efgh')
    await userEvent.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() => expect(screen.getByTestId('login-retry')).toBeInTheDocument())
  })
})
