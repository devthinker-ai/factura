import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PeppolLedgerPage } from '@/pages/PeppolLedgerPage'
import { PeppolSettingsPage } from '@/pages/PeppolSettingsPage'
import { InvoiceDetailPage } from '@/pages/InvoiceDetailPage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import type { PeppolMessage, PeppolStatus, Row } from '@/lib/api'
import { ApiError } from '@/lib/api'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    peppolMessages: vi.fn(),
    peppolMessage: vi.fn(),
    peppolStatus: vi.fn(),
    peppolParticipants: vi.fn(),
    peppolPing: vi.fn(),
    peppolSend: vi.fn(),
    get: vi.fn(),
    fetchOriginalBlob: vi.fn(),
  }
})

import * as api from '@/lib/api'

const sampleMessages: PeppolMessage[] = [
  {
    id: 1,
    direction: 'in',
    from_peppol_id: 'DE:A',
    to_peppol_id: 'DE:SELF',
    ap_message_id: 'in-1',
    status: 'delivered',
    invoice_id: 10,
    created_at: '2026-09-10T10:00:00Z',
  },
  {
    id: 2,
    direction: 'out',
    from_peppol_id: 'DE:SELF',
    to_peppol_id: 'DE:B',
    ap_message_id: 'out-1',
    status: 'failed',
    invoice_id: 11,
    created_at: '2026-09-10T11:00:00Z',
  },
]

describe('PeppolLedgerPage', () => {
  beforeEach(() => {
    vi.mocked(api.peppolMessages).mockImplementation(async (p) => {
      if (p?.direction === 'in') return [sampleMessages[0]]
      if (p?.direction === 'out') return [sampleMessages[1]]
      return sampleMessages
    })
  })

  it('renders inbound/outbound with status badges and invoice links', async () => {
    render(
      <TestProviders>
        <MemoryRouter basename="/app" initialEntries={['/app/peppol']}>
          <AuthProvider>
            <Routes>
              <Route path="/peppol" element={<PeppolLedgerPage />} />
              <Route path="/invoices/:id" element={<div>detail</div>} />
              <Route path="/settings/peppol" element={<div>settings</div>} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByText('DE:A → DE:SELF')).toBeInTheDocument()
    expect(screen.getByText('delivered')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '#10' })).toHaveAttribute('href', '/app/invoices/10')

    await userEvent.click(screen.getByRole('tab', { name: /Outbound/i }))
    expect(await screen.findByText('DE:SELF → DE:B')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
  })
})

describe('PeppolSettingsPage', () => {
  it('shows configured ✓/✗ from status', async () => {
    vi.mocked(api.peppolStatus).mockResolvedValue({
      self_id: 'DE:SELF',
      ap_name: 'fake',
      ap_configured: true,
      callback_url: 'https://example/peppol/inbound',
      poll_enabled: false,
      counts: {},
    } satisfies PeppolStatus)
    vi.mocked(api.peppolParticipants).mockResolvedValue([
      { peppol_id: 'DE:SELF', service_type: 'buyer-seller', name: 'Self', is_self: true },
    ])

    render(
      <TestProviders>
        <MemoryRouter>
          <AuthProvider>
            <PeppolSettingsPage />
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByText('configured ✓')).toBeInTheDocument()
    expect(screen.getByText('fake')).toBeInTheDocument()
  })
})

describe('InvoiceDetailPage Peppol send', () => {
  function row(partial: Partial<Row> = {}): Row {
    return {
      id: 42,
      external_id: 'ext-42',
      format: 'xrechnung-cii',
      status: 'valid',
      vendor_name: 'Vendor Co',
      invoice_number: 'XR-1',
      buyer_name: 'Buyer',
      invoice_date: '2026-02-01',
      total: 100,
      vat_amount: 19,
      currency: 'EUR',
      created_at: '2026-02-01T12:00:00Z',
      report_json: { valid: true, summary: 'ok', violations: [] },
      ...partial,
    }
  }

  beforeEach(() => {
    vi.mocked(api.get).mockResolvedValue(row())
    vi.mocked(api.peppolParticipants).mockResolvedValue([
      { peppol_id: 'DE:BUYER', service_type: 'buyer', name: 'Buyer', is_self: false },
    ])
  })

  it('Send via Peppol shows 201 receipt card', async () => {
    vi.mocked(api.peppolSend).mockResolvedValue({
      message_id: 9,
      status: 'delivered',
      invoice_id: 42,
      receipt: { ap_message_id: 'ap-9', status: 'delivered' },
    })
    render(
      <TestProviders>
        <MemoryRouter basename="/app" initialEntries={['/app/invoices/42']}>
          <AuthProvider>
            <Routes>
              <Route path="/invoices/:id" element={<InvoiceDetailPage />} />
              <Route path="/peppol" element={<div>ledger</div>} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    await screen.findByText('Vendor Co')
    await userEvent.click(screen.getByRole('button', { name: /Send via Peppol/i }))
    const input = await screen.findByLabelText(/To Peppol ID/i)
    await userEvent.type(input, 'DE:BUYER')
    await userEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    await waitFor(() => expect(screen.getByTestId('peppol-send-ok')).toBeInTheDocument())
    expect(screen.getByText(/message #9/)).toBeInTheDocument()
  })

  it('424 shows AP rejection detail', async () => {
    vi.mocked(api.peppolSend).mockRejectedValue(
      new ApiError(424, { error: 'ap rejected', detail: 'failed' }),
    )
    render(
      <TestProviders>
        <MemoryRouter basename="/app" initialEntries={['/app/invoices/42']}>
          <AuthProvider>
            <Routes>
              <Route path="/invoices/:id" element={<InvoiceDetailPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    await screen.findByText('Vendor Co')
    await userEvent.click(screen.getByRole('button', { name: /Send via Peppol/i }))
    await userEvent.type(await screen.findByLabelText(/To Peppol ID/i), 'DE:BUYER')
    await userEvent.click(screen.getByRole('button', { name: /^Send$/i }))
    await waitFor(() => expect(screen.getByTestId('peppol-send-err')).toBeInTheDocument())
    expect(screen.getByText('AP rejected')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
  })
})
