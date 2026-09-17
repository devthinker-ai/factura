import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { InvoiceDetailPage } from '@/pages/InvoiceDetailPage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import type { Row } from '@/lib/api'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    get: vi.fn(),
    fetchOriginalBlob: vi.fn(),
    evidence: vi.fn().mockResolvedValue({
      invoice_id: 42,
      hash: 'abc123',
      prev_hash: 'genesis',
      created_at: '2026-02-01T12:00:00Z',
      verified: true,
    }),
    audit: vi.fn(),
  }
})

import * as api from '@/lib/api'

const human =
  'USt-IdNr des Käufers fehlt bei reverse-charge / buyer VAT ID required for reverse charge'

function row(partial: Partial<Row>): Row {
  return {
    id: 42,
    external_id: 'ext-42',
    format: 'xrechnung-cii',
    status: 'invalid',
    vendor_name: 'Vendor Co',
    invoice_number: 'XR-1',
    buyer_name: 'Buyer',
    invoice_date: '2026-02-01',
    total: 100,
    vat_amount: 0,
    currency: 'EUR',
    created_at: '2026-02-01T12:00:00Z',
    ...partial,
  }
}

function renderDetail(id = '42', lang: 'en' | 'de' = 'en') {
  return render(
    <TestProviders lang={lang}>
      <MemoryRouter basename="/app" initialEntries={[`/app/invoices/${id}`]}>
        <AuthProvider>
          <Routes>
            <Route path="/invoices/:id" element={<InvoiceDetailPage />} />
            <Route path="/" element={<div>inbox</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </TestProviders>,
  )
}

describe('InvoiceDetailPage', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
  })

  it('renders violation human verbatim', async () => {
    vi.mocked(api.get).mockResolvedValue(
      row({
        report_json: {
          valid: false,
          level: 'business',
          summary: '1 violation',
          violations: [{ rule_id: 'BR-AE-2', human, value: 'missing' }],
        },
      }),
    )
    renderDetail()
    expect(await screen.findByText(human)).toBeInTheDocument()
    expect(screen.getByText('BR-AE-2')).toBeInTheDocument()
  })

  it('verbatim human unchanged when switching DE→EN', async () => {
    const marker = 'DE_ONLY_MARKER / EN_SIDE_MARKER'
    vi.mocked(api.get).mockResolvedValue(
      row({
        report_json: {
          valid: false,
          violations: [{ rule_id: 'BR-TEST', human: marker }],
        },
      }),
    )
    const { rerender } = render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/invoices/42']}>
          <AuthProvider>
            <Routes>
              <Route path="/invoices/:id" element={<InvoiceDetailPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByText(marker)).toBeInTheDocument()
    const before = screen.getByText(marker).textContent
    rerender(
      <TestProviders lang="en">
        <MemoryRouter basename="/app" initialEntries={['/app/invoices/42']}>
          <AuthProvider>
            <Routes>
              <Route path="/invoices/:id" element={<InvoiceDetailPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByText(marker)).toBeInTheDocument()
    expect(screen.getByText(marker).textContent).toBe(before)
  })

  it('shows parse_error reason prominently', async () => {
    vi.mocked(api.get).mockResolvedValue(
      row({
        status: 'parse_error',
        report_json: {
          valid: false,
          level: 'schema',
          summary: 'not an invoice XML at all',
          violations: [],
        },
      }),
    )
    renderDetail()
    expect(await screen.findByText(/parse error/i)).toBeInTheDocument()
    expect(screen.getByText('not an invoice XML at all')).toBeInTheDocument()
  })

  it('Copy JSON writes clipboard', async () => {
    const user = userEvent.setup()
    const doc = { $schema: 'https://gobl.org/draft-0/bill/invoice', code: 'X' }
    vi.mocked(api.get).mockResolvedValue(
      row({
        doc_gobl_json: doc,
        status: 'valid',
        report_json: { valid: true, violations: [], summary: 'ok' },
      }),
    )
    renderDetail()
    await screen.findByText('Vendor Co')
    await user.click(screen.getByRole('button', { name: /copy json/i }))
    await waitFor(async () => {
      expect(await navigator.clipboard.readText()).toBe(JSON.stringify(doc, null, 2))
    })
  })

  it('404 message', async () => {
    vi.mocked(api.get).mockRejectedValue(new api.ApiError(404, { error: 'not found' }))
    renderDetail('99')
    expect(await screen.findByText(/Invoice 99 not found/i)).toBeInTheDocument()
  })

  it('copies rule id on click', async () => {
    const user = userEvent.setup()
    vi.mocked(api.get).mockResolvedValue(
      row({
        report_json: {
          valid: false,
          violations: [{ rule_id: 'BR-DE-15', human: 'buyer ref missing' }],
        },
      }),
    )
    renderDetail()
    await screen.findByText('BR-DE-15')
    await user.click(screen.getByTestId('rule-id-BR-DE-15'))
    await waitFor(async () => {
      expect(await navigator.clipboard.readText()).toBe('BR-DE-15')
    })
  })

  it('shows invoice_type chip and GoBD evidence', async () => {
    const user = userEvent.setup()
    vi.mocked(api.get).mockResolvedValue(
      row({
        invoice_type: '381',
        status: 'valid',
        report_json: { valid: true, violations: [], summary: 'ok' },
      }),
    )
    vi.mocked(api.audit).mockResolvedValue({
      ok: true,
      link_count: 3,
      head: 'deadbeef',
      first_broken_id: 0,
    })
    renderDetail()
    expect(await screen.findByTestId('invoice-type')).toHaveTextContent('381')
    expect(screen.getByTestId('gobd-evidence')).toBeInTheDocument()
    expect(screen.getByText('verified')).toBeInTheDocument()
    expect(screen.getByText('abc123')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /verify archive/i }))
    expect(await screen.findByTestId('audit-summary')).toHaveTextContent(/Archive OK/)
  })

  it('shows broken audit summary', async () => {
    const user = userEvent.setup()
    vi.mocked(api.get).mockResolvedValue(
      row({
        invoice_type: '380',
        status: 'valid',
        report_json: { valid: true, violations: [] },
      }),
    )
    vi.mocked(api.audit).mockResolvedValue({
      ok: false,
      link_count: 2,
      head: '',
      first_broken_id: 7,
    })
    renderDetail()
    await screen.findByTestId('gobd-evidence')
    await user.click(screen.getByRole('button', { name: /verify archive/i }))
    expect(await screen.findByTestId('audit-summary')).toHaveTextContent(/broken at invoice 7/)
  })
})
