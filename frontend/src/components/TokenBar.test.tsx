import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TokenBar } from '@/components/TokenBar'
import { renderWithProviders } from '@/test/render'
import { clearToken, fetchJson } from '@/lib/api'

describe('TokenBar', () => {
  beforeEach(() => {
    clearToken()
    localStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('appears on 401 without token and saves + retries (token mode)', async () => {
    const user = userEvent.setup()
    const retry = vi.fn()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/auth-status')) {
          return new Response(JSON.stringify({ mode: 'token' }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return {
          ok: false,
          status: 401,
          headers: new Headers({ 'content-type': 'application/json' }),
          json: async () => ({ error: 'unauthorized' }),
        } as Response
      }),
    )

    renderWithProviders(<TokenBar onRetry={retry} />)
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled())

    await expect(fetchJson('/invoices')).rejects.toMatchObject({ status: 401 })
    expect(await screen.findByTestId('token-bar')).toBeInTheDocument()

    await user.type(screen.getByLabelText(/api token/i), 'secret-tok')
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(localStorage.getItem('factura_token')).toBe('secret-tok')
    expect(retry).toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByTestId('token-bar')).not.toBeInTheDocument()
    })
  })
})
