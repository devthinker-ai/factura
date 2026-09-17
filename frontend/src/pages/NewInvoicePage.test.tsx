import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewInvoicePage } from '@/pages/NewInvoicePage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    generate: vi.fn(),
    ingest: vi.fn(),
    listCompanies: vi.fn().mockResolvedValue([]),
  }
})

import * as api from '@/lib/api'

const downloadSpy = vi.fn()

vi.mock('@/lib/utils', async () => {
  const actual = await vi.importActual<typeof import('@/lib/utils')>('@/lib/utils')
  return {
    ...actual,
    downloadBlob: (...args: unknown[]) => downloadSpy(...args),
  }
})

function renderNew() {
  return render(
    <TestProviders>
      <MemoryRouter basename="/app" initialEntries={['/app/new']}>
        <AuthProvider>
          <Routes>
            <Route path="/new" element={<NewInvoicePage />} />
            <Route path="/invoices/:id" element={<div>detail</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </TestProviders>,
  )
}

describe('NewInvoicePage', () => {
  beforeEach(() => {
    vi.mocked(api.generate).mockReset()
    vi.mocked(api.ingest).mockReset()
    downloadSpy.mockReset()
  })

  it('generate downloads XR and ZUGFeRD', async () => {
    const user = userEvent.setup()
    const xml = '<?xml version="1.0"?><Invoice/>'
    const pdfB64 = btoa('%PDF-1.4')
    vi.mocked(api.generate).mockResolvedValue({
      xrechnung: xml,
      zugferd_pdf_base64: pdfB64,
      self_check: true,
      filename_xrechnung: 'R-1.xml',
      filename_zugferd: 'R-1.pdf',
    })
    vi.mocked(api.ingest).mockResolvedValue({
      id: 55,
      external_id: 'R-1',
      status: 'valid',
      format: 'xrechnung-cii',
    })
    renderNew()
    await user.type(screen.getByLabelText('Invoice number'), 'R-1')
    await user.click(screen.getByRole('button', { name: /generate & download/i }))
    await screen.findByTestId('generate-result')

    await user.click(screen.getByRole('button', { name: /download xrechnung/i }))
    expect(downloadSpy).toHaveBeenCalledWith(expect.any(Blob), 'R-1.xml')
    await user.click(screen.getByRole('button', { name: /download zugferd/i }))
    expect(downloadSpy).toHaveBeenCalledWith(expect.any(Blob), 'R-1.pdf')
  })

  it('shows 422 error text as-is', async () => {
    const user = userEvent.setup()
    vi.mocked(api.generate).mockRejectedValue(
      new api.ApiError(422, { error: 'supplier tax_id invalid: boom' }),
    )
    renderNew()
    await user.click(screen.getByRole('button', { name: /generate & download/i }))
    expect(await screen.findByText('supplier tax_id invalid: boom')).toBeInTheDocument()
  })

  it('archive checkbox POSTs invoices with XML', async () => {
    const user = userEvent.setup()
    const xml = '<CrossIndustryInvoice/>'
    vi.mocked(api.generate).mockResolvedValue({
      xrechnung: xml,
      zugferd_pdf_base64: '',
      self_check: true,
      filename_xrechnung: 'R-2.xml',
      filename_zugferd: '',
    })
    vi.mocked(api.ingest).mockResolvedValue({
      id: 77,
      external_id: 'R-2',
      status: 'valid',
      format: 'xrechnung-cii',
    })
    renderNew()
    await user.type(screen.getByLabelText('Invoice number'), 'R-2')
    expect(screen.getByLabelText(/and archive it/i)).toBeChecked()
    await user.click(screen.getByRole('button', { name: /generate & download/i }))
    await waitFor(() => expect(api.ingest).toHaveBeenCalled())
    const [body, opts] = vi.mocked(api.ingest).mock.calls[0]
    expect(opts).toEqual(
      expect.objectContaining({ externalId: 'R-2', source: 'web-generate' }),
    )
    const decoded = ArrayBuffer.isView(body)
      ? new TextDecoder().decode(body as ArrayBufferView)
      : body instanceof ArrayBuffer
        ? new TextDecoder().decode(body)
        : String(body)
    expect(decoded).toContain('CrossIndustryInvoice')
  })

  it('prefills seller from default company', async () => {
    vi.mocked(api.listCompanies).mockResolvedValue([
      {
        id: 1,
        name: 'Prefill GmbH',
        seller_vat_id: 'DE123456789',
        is_default: true,
        created_at: '2026-09-10T00:00:00Z',
      },
    ])
    renderNew()
    await waitFor(() => {
      expect(screen.getByDisplayValue('Prefill GmbH')).toBeInTheDocument()
    })
    expect(screen.getByDisplayValue('DE123456789')).toBeInTheDocument()
  })

  it('reveals preceding fields for credit note', async () => {
    const user = userEvent.setup()
    renderNew()
    expect(screen.queryByLabelText(/Preceding invoice number/i)).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText(/Document type/i), 'credit-note')
    expect(screen.getByLabelText(/Preceding invoice number/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Preceding issue date/i)).toBeInTheDocument()
  })

  it('shows 422 detail verbatim for missing preceding', async () => {
    const user = userEvent.setup()
    const detail =
      'FACTURA-PRECEDING-REQUIRED: Vorhergehende Rechnung (BG-3) fehlt / preceding invoice reference (BG-3) required'
    vi.mocked(api.generate).mockRejectedValue(
      new api.ApiError(422, { error: 'generate failed', detail }),
    )
    renderNew()
    await user.selectOptions(screen.getByLabelText(/Document type/i), 'credit-note')
    await user.click(screen.getByRole('button', { name: /generate & download/i }))
    expect(await screen.findByText(detail)).toBeInTheDocument()
  })

  it('renders Leitweg-ID field in DE', async () => {
    render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/new']}>
          <AuthProvider>
            <Routes>
              <Route path="/new" element={<NewInvoicePage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByLabelText(/Leitweg-ID/i)).toBeInTheDocument()
  })

  it('payment method toggle swaps Überweisung / Lastschrift fields', async () => {
    const user = userEvent.setup()
    renderNew()
    expect(screen.getByLabelText(/^IBAN$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/BIC/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/Mandate reference/i)).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText(/Payment method/i), 'direct-debit')
    expect(screen.getByLabelText(/Mandate reference/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Creditor ID/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/BIC/i)).not.toBeInTheDocument()
  })

  it('Kleinunternehmer toggle swaps USt-IdNr for Steuernummer', async () => {
    const user = userEvent.setup()
    renderNew()
    expect(screen.getAllByLabelText(/USt-IdNr/i).length).toBeGreaterThan(0)
    await user.click(screen.getByLabelText(/Kleinunternehmer/i))
    expect(screen.getByLabelText(/Steuernummer|Tax number/i)).toBeInTheDocument()
  })

  it('renders payment + contact fields in DE', async () => {
    render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/new']}>
          <AuthProvider>
            <Routes>
              <Route path="/new" element={<NewInvoicePage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findByLabelText(/Zahlungsart/i)).toBeInTheDocument()
    expect(screen.getAllByLabelText(/Ansprechpartner/i).length).toBeGreaterThan(0)
    expect(screen.getByLabelText(/Telefon \(BT-42/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Bereits gezahlt/i)).toBeInTheDocument()
  })

  it('rejects disallowed attachment MIME client-side', async () => {
    const user = userEvent.setup()
    renderNew()
    const input = screen.getByLabelText(/Attachments/i)
    const bad = new File(['<html></html>'], 'page.html', { type: 'text/html' })
    await user.upload(input, bad)
    expect(screen.queryByText('page.html')).not.toBeInTheDocument()
  })

  it('optional refs/period/discounts sections collapse; expand reveals fields (EN)', async () => {
    const user = userEvent.setup()
    renderNew()
    expect(screen.queryByLabelText(/Project \(BT-11\)/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/Delivery \/ supply date/i)).not.toBeInTheDocument()
    await user.click(screen.getByTestId('section-refs'))
    expect(screen.getByLabelText(/Project \(BT-11\)/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Purchase order \(BT-13\)/i)).toBeInTheDocument()
    await user.click(screen.getByTestId('section-period'))
    expect(screen.getByLabelText(/Delivery \/ supply date/i)).toBeInTheDocument()
    await user.click(screen.getByTestId('section-discounts'))
    expect(screen.getByRole('button', { name: /Add discount/i })).toBeInTheDocument()
  })

  it('renders optional sections + party IDs + country in DE', async () => {
    const user = userEvent.setup()
    render(
      <TestProviders lang="de">
        <MemoryRouter basename="/app" initialEntries={['/app/new']}>
          <AuthProvider>
            <Routes>
              <Route path="/new" element={<NewInvoicePage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </TestProviders>,
    )
    expect(await screen.findAllByLabelText(/Land \(BT-40/i)).toHaveLength(2)
    expect(screen.getByLabelText(/Lieferantennummer/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Kundennummer/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Handelsregisternummer/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Verwendungszweck/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Zahlungsbedingungen/i)).toBeInTheDocument()
    await user.click(screen.getByTestId('section-refs'))
    expect(screen.getByLabelText(/Projekt \(BT-11\)/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/Bestellnummer/i)).toBeInTheDocument()
    await user.click(screen.getByTestId('section-period'))
    expect(screen.getByLabelText(/Liefer-\/Leistungsdatum/i)).toBeInTheDocument()
  })
})
