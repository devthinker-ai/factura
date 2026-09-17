import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { LicensePage } from '@/pages/LicensePage'
import { PeppolLedgerPage } from '@/pages/PeppolLedgerPage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import { ApiError, type LicenseInfo } from '@/lib/api'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getLicense: vi.fn(),
    postLicense: vi.fn(),
    deleteLicense: vi.fn(),
    listCompanies: vi.fn(),
    addCompany: vi.fn(),
    setDefaultCompany: vi.fn(),
    listAPIKeys: vi.fn(),
    createAPIKey: vi.fn(),
    patchAPIKey: vi.fn(),
    deleteAPIKey: vi.fn(),
    peppolMessages: vi.fn(),
    generate: vi.fn(),
  }
})

import * as api from '@/lib/api'

const soloLicense: LicenseInfo = {
  plan: 'solo',
  licensed: false,
  subject: '',
  expires: '',
  caps: { max_companies: 1, max_api_keys: 2, max_clients: 0, peppol: false },
  in_use: { companies: 0, api_keys: 0, invoices_this_month: 0 },
}

const proLicense: LicenseInfo = {
  plan: 'pro',
  licensed: true,
  subject: 'buyer@example.com',
  expires: '2027-01-01T00:00:00Z',
  caps: { max_companies: 10, max_api_keys: 20, max_clients: 10, peppol: true },
  in_use: { companies: 1, api_keys: 1, invoices_this_month: 12 },
}

describe('LicensePage', () => {
  beforeEach(() => {
    vi.mocked(api.getLicense).mockResolvedValue(soloLicense)
    vi.mocked(api.listCompanies).mockResolvedValue([])
    vi.mocked(api.listAPIKeys).mockResolvedValue([])
  })

  it('renders unlicensed Solo trial state', async () => {
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await waitFor(() => {
      expect(screen.getByText('Solo (trial)')).toBeInTheDocument()
    })
    expect(screen.getByRole('heading', { name: /Settings → License/i })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/Paste license key/i)).toBeInTheDocument()
    expect(screen.getByText('0/1')).toBeInTheDocument() // companies in-use/max
  })

  it('applies valid key and shows Pro', async () => {
    const user = userEvent.setup()
    vi.mocked(api.postLicense).mockResolvedValue(proLicense)
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText('Solo (trial)')
    await user.type(screen.getByPlaceholderText(/Paste license key/i), 'eyJ.fake.jwt')
    await user.click(screen.getByRole('button', { name: /Apply key/i }))
    await waitFor(() => {
      expect(screen.getByText('pro')).toBeInTheDocument()
    })
    expect(screen.getByText('buyer@example.com')).toBeInTheDocument()
  })

  it('shows error toast detail on invalid key', async () => {
    const user = userEvent.setup()
    vi.mocked(api.postLicense).mockRejectedValue(
      new ApiError(422, { error: 'invalid_license_key', detail: 'token is expired' }),
    )
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText('Solo (trial)')
    await user.type(screen.getByPlaceholderText(/Paste license key/i), 'bad')
    await user.click(screen.getByRole('button', { name: /Apply key/i }))
    await waitFor(() => {
      expect(screen.getByText(/Invalid license key/i)).toBeInTheDocument()
      expect(screen.getByText(/token is expired/i)).toBeInTheDocument()
    })
  })

  it('shows DE chrome labels', async () => {
    render(
      <MemoryRouter>
        <TestProviders lang="de">
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /Einstellungen → Lizenz/i })).toBeInTheDocument()
    })
    expect(screen.getByText('Solo (Test)')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/Lizenzschlüssel/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Schlüssel anwenden/i })).toBeInTheDocument()
  })

  it('creates API key and shows plaintext once', async () => {
    const user = userEvent.setup()
    vi.mocked(api.createAPIKey).mockResolvedValue({
      key: 'factura_aabbccddeeff001122334455',
      name: 'acme',
      rpm: 30,
      monthly_invoices: 1000,
    })
    vi.mocked(api.listAPIKeys).mockResolvedValue([
      {
        id: 'factura_aabbccddeeff001122334455',
        name: 'acme',
        rpm: 30,
        monthly_invoices: 1000,
        invoices_this_month: 3,
        enabled: true,
        created_at: '2026-09-10T00:00:00Z',
      },
    ])
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText('Solo (trial)')
    const keyName = document.getElementById('key-name') as HTMLInputElement
    await user.type(keyName, 'acme')
    await user.click(screen.getByRole('button', { name: /Create key/i }))
    await waitFor(() => {
      expect(screen.getByTestId('plaintext-key')).toHaveTextContent('factura_aabbccddeeff001122334455')
    })
    expect(screen.getByText(/Shown only once/i)).toBeInTheDocument()
  })

  it('shows plan-limit banner when second company fails on Solo', async () => {
    const user = userEvent.setup()
    vi.mocked(api.listCompanies).mockResolvedValue([
      {
        id: 1,
        name: 'First',
        seller_vat_id: 'DE1',
        is_default: true,
        created_at: '2026-09-10T00:00:00Z',
      },
    ])
    vi.mocked(api.addCompany).mockRejectedValue(
      new ApiError(403, { error: 'plan_limit', detail: 'Solo allows 1 company' }),
    )
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText('First')
    await user.type(document.getElementById('co-name') as HTMLInputElement, 'Second GmbH')
    await user.click(screen.getByRole('button', { name: /Add company/i }))
    await waitFor(() => {
      expect(screen.getByTestId('plan-limit-banner')).toHaveTextContent(/Solo allows 1 company/)
    })
  })

  it('removes license back to Solo trial', async () => {
    const user = userEvent.setup()
    vi.mocked(api.getLicense).mockResolvedValue(proLicense)
    vi.mocked(api.deleteLicense).mockResolvedValue(undefined)
    vi.mocked(api.getLicense)
      .mockResolvedValueOnce(proLicense)
      .mockResolvedValue(soloLicense)
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <LicensePage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText('pro')
    await user.click(screen.getByRole('button', { name: /Remove key/i }))
    await waitFor(() => {
      expect(api.deleteLicense).toHaveBeenCalled()
    })
  })
})

describe('Peppol plan-limit empty state', () => {
  it('shows requires Pro without error loop', async () => {
    vi.mocked(api.peppolMessages).mockRejectedValue(
      new ApiError(403, { error: 'plan_limit', detail: 'Peppol requires the Pro plan' }),
    )
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <PeppolLedgerPage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await waitFor(() => {
      expect(screen.getByText(/Peppol requires Pro/i)).toBeInTheDocument()
    })
    expect(screen.queryByText(/Failed to load Peppol/i)).not.toBeInTheDocument()
  })
})
