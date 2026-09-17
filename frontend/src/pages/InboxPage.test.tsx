import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { InboxPage } from '@/pages/InboxPage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import type { Row } from '@/lib/api'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    list: vi.fn(),
    vendors: vi.fn(),
    exportCsv: vi.fn(),
    ingest: vi.fn(),
    health: vi.fn(),
    peppolStatus: vi.fn(),
  }
})

import * as api from '@/lib/api'

const sampleRows: Row[] = [
  {
    id: 1,
    external_id: 'a',
    format: 'xrechnung-cii',
    status: 'valid',
    vendor_name: 'Acme GmbH',
    invoice_number: 'INV-1',
    buyer_name: 'Buyer',
    invoice_date: '2026-01-15',
    total: 119,
    vat_amount: 19,
    currency: 'EUR',
    created_at: '2026-01-15T10:00:00Z',
    source: 'mail',
  },
  {
    id: 2,
    external_id: 'b',
    format: 'xrechnung-cii',
    status: 'invalid',
    vendor_name: 'Beta AG',
    invoice_number: 'INV-2',
    buyer_name: 'Buyer',
    invoice_date: '2026-01-16',
    total: 50,
    vat_amount: 0,
    currency: 'EUR',
    created_at: '2026-01-16T10:00:00Z',
  },
]

function renderInbox() {
  return render(
    <TestProviders>
      <MemoryRouter basename="/app" initialEntries={['/app/']}>
        <AuthProvider>
          <Routes>
            <Route path="/" element={<InboxPage />} />
            <Route path="/invoices/:id" element={<div>detail</div>} />
            <Route path="/new" element={<div>new</div>} />
            <Route path="/settings/peppol" element={<div>settings</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </TestProviders>,
  )
}

async function uploadFile(file: File) {
  fireEvent.drop(screen.getByTestId('inbox-dropzone'), {
    dataTransfer: { files: [file], types: ['Files'] },
  })
}

describe('InboxPage', () => {
  beforeEach(() => {
    vi.mocked(api.list).mockReset()
    vi.mocked(api.vendors).mockReset()
    vi.mocked(api.exportCsv).mockReset()
    vi.mocked(api.ingest).mockReset()
    vi.mocked(api.health).mockReset()
    vi.mocked(api.peppolStatus).mockReset()
    vi.mocked(api.vendors).mockResolvedValue(['Acme GmbH', 'Beta AG'])
    vi.mocked(api.list).mockResolvedValue({ rows: sampleRows, total: 2 })
    vi.mocked(api.health).mockResolvedValue({ status: 'ok', version: 'test' })
    vi.mocked(api.peppolStatus).mockResolvedValue({
      self_id: '',
      ap_name: 'fake',
      ap_configured: true,
    })
  })

  it('renders rows with status badges', async () => {
    renderInbox()
    expect(await screen.findByText('INV-1')).toBeInTheDocument()
    const table = screen.getByRole('table')
    expect(within(table).getByText('valid')).toBeInTheDocument()
    expect(within(table).getByText('invalid')).toBeInTheDocument()
  })

  it('passes status=invalid in list query', async () => {
    const user = userEvent.setup()
    renderInbox()
    await screen.findByText('INV-1')
    await user.selectOptions(screen.getByLabelText('Status'), 'invalid')
    await waitFor(() => {
      expect(api.list).toHaveBeenCalledWith(
        expect.objectContaining({ status: 'invalid', offset: 0 }),
      )
    })
  })

  it('Load older increments offset', async () => {
    const user = userEvent.setup()
    vi.mocked(api.list).mockImplementation(async (params) => {
      if ((params?.offset ?? 0) === 0) {
        return { rows: sampleRows, total: 100 }
      }
      return {
        rows: [
          {
            ...sampleRows[0],
            id: 99,
            invoice_number: 'OLD-99',
          },
        ],
        total: 100,
      }
    })
    renderInbox()
    await screen.findByText('INV-1')
    await user.click(screen.getByRole('button', { name: /load older/i }))
    await waitFor(() => {
      expect(api.list).toHaveBeenCalledWith(expect.objectContaining({ offset: 50 }))
    })
    expect(await screen.findByText('OLD-99')).toBeInTheDocument()
  })

  it('export uses current filters', async () => {
    const user = userEvent.setup()
    vi.mocked(api.exportCsv).mockResolvedValue(new Blob(['a,b']))
    renderInbox()
    await screen.findByText('INV-1')
    await user.selectOptions(screen.getByLabelText('Status'), 'invalid')
    await waitFor(() =>
      expect(api.list).toHaveBeenCalledWith(expect.objectContaining({ status: 'invalid' })),
    )
    await user.click(screen.getByRole('button', { name: /export csv/i }))
    await waitFor(() => {
      expect(api.exportCsv).toHaveBeenCalledWith(expect.objectContaining({ status: 'invalid' }))
    })
  })

  it('201 ingest refetches list', async () => {
    vi.mocked(api.ingest).mockResolvedValue({
      id: 10,
      external_id: '',
      status: 'valid',
      format: 'xrechnung-cii',
    })
    renderInbox()
    await screen.findByText('INV-1')
    const callsBefore = vi.mocked(api.list).mock.calls.length
    await uploadFile(new File(['<xml/>'], 'inv.xml', { type: 'application/xml' }))
    await waitFor(() => expect(api.ingest).toHaveBeenCalled())
    await waitFor(() => {
      expect(vi.mocked(api.list).mock.calls.length).toBeGreaterThan(callsBefore)
    })
    expect(api.ingest).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ source: 'web-upload' }),
    )
  })

  it('409 shows duplicate toast', async () => {
    vi.mocked(api.ingest).mockRejectedValue(
      new api.ApiError(409, { error: 'external_id already exists', id: 7 }),
    )
    renderInbox()
    await screen.findByText('INV-1')
    await uploadFile(new File(['x'], 'dup.xml', { type: 'application/xml' }))
    expect(await screen.findByText(/duplicate \(id 7\)/i)).toBeInTheDocument()
  })

  it('415 shows parse_error link', async () => {
    vi.mocked(api.ingest).mockRejectedValue(
      new api.ApiError(415, {
        error: 'unrecognized invoice format',
        detail: 'nope',
        id: 8,
      }),
    )
    renderInbox()
    await screen.findByText('INV-1')
    await uploadFile(new File(['x'], 'bad.bin', { type: 'application/octet-stream' }))
    const toastEl = await screen.findByText(/kept as parse_error/i)
    expect(within(toastEl.closest('div')!).getByRole('link', { name: '8' })).toHaveAttribute(
      'href',
      '/app/invoices/8',
    )
  })

  it('shows onboarding when empty and Peppol unconfigured', async () => {
    vi.mocked(api.list).mockResolvedValue({ rows: [], total: 0 })
    vi.mocked(api.peppolStatus).mockResolvedValue({
      self_id: '',
      ap_name: '',
      ap_configured: false,
    })
    localStorage.removeItem('factura:onboarding-dismissed')
    renderInbox()
    expect(await screen.findByTestId('onboarding-panel')).toBeInTheDocument()
  })

  it('never shows onboarding when archive has rows', async () => {
    localStorage.removeItem('factura:onboarding-dismissed')
    renderInbox()
    await screen.findByText('INV-1')
    expect(screen.queryByTestId('onboarding-panel')).not.toBeInTheDocument()
  })

  it('dismisses onboarding and remembers', async () => {
    const user = userEvent.setup()
    vi.mocked(api.list).mockResolvedValue({ rows: [], total: 0 })
    vi.mocked(api.peppolStatus).mockResolvedValue({
      self_id: '',
      ap_name: '',
      ap_configured: false,
    })
    localStorage.removeItem('factura:onboarding-dismissed')
    renderInbox()
    expect(await screen.findByTestId('onboarding-panel')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /got it/i }))
    expect(screen.queryByTestId('onboarding-panel')).not.toBeInTheDocument()
    expect(localStorage.getItem('factura:onboarding-dismissed')).toBe('1')
  })

  it('error state renders Retry and recovers', async () => {
    const user = userEvent.setup()
    vi.mocked(api.list)
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({ rows: sampleRows, total: 2 })
    renderInbox()
    expect(await screen.findByTestId('error-state')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(await screen.findByText('INV-1')).toBeInTheDocument()
  })

  it('keyboard: arrow moves focus, Enter opens detail', async () => {
    const user = userEvent.setup()
    renderInbox()
    await screen.findByText('INV-1')
    const row1 = screen.getByTestId('inbox-row-1')
    row1.focus()
    await user.keyboard('{ArrowDown}')
    expect(screen.getByTestId('inbox-row-2')).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(await screen.findByText('detail')).toBeInTheDocument()
  })
})
