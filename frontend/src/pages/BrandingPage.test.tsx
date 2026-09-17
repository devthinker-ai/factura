import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BrandingPage } from '@/pages/BrandingPage'
import { TestProviders } from '@/test/render'
import { AuthProvider } from '@/components/AuthProvider'
import { MemoryRouter } from 'react-router-dom'
import de from '@/locales/de.json'
import en from '@/locales/en.json'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    listCompanies: vi.fn(),
    getCompany: vi.fn(),
    addCompany: vi.fn(),
    patchCompanyBranding: vi.fn(),
    generate: vi.fn(),
  }
})

import * as api from '@/lib/api'

const company = {
  id: 1,
  name: 'Brand GmbH',
  is_default: true,
  created_at: '2026-09-01T00:00:00Z',
  has_logo: false,
  branding: {},
}

function setFile(input: HTMLElement, file: File) {
  fireEvent.change(input, { target: { files: [file] } })
}

describe('BrandingPage', () => {
  beforeEach(() => {
    vi.mocked(api.listCompanies).mockResolvedValue([company])
    vi.mocked(api.getCompany).mockResolvedValue({ ...company, branding: {} })
    vi.mocked(api.patchCompanyBranding).mockResolvedValue({
      ...company,
      has_logo: true,
      branding: { logo_b64: 'abc', logo_mime: 'image/jpeg', accent: '#1E5AA8' },
    })
    vi.mocked(api.generate).mockResolvedValue({
      xrechnung: '',
      zugferd_pdf_base64: 'JVBERi0x',
      self_check: true,
      filename_xrechnung: 'x.xml',
      filename_zugferd: 'z.pdf',
    })
  })

  it('rejects non-image and oversized files with destructive toast', async () => {
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <BrandingPage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByText(/Settings → Branding/)
    const input = await screen.findByTestId('branding-logo-input')

    setFile(input, new File(['hello'], 'x.txt', { type: 'text/plain' }))
    await waitFor(() => {
      expect(document.body.textContent).toMatch(/PNG or JPEG/)
    })

    setFile(input, new File([new Uint8Array(512_001)], 'big.jpg', { type: 'image/jpeg' }))
    await waitFor(() => {
      expect(document.body.textContent).toMatch(/512 KB/)
    })
  })

  it('saves branding via PATCH with logo_b64', async () => {
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <BrandingPage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    const input = await screen.findByTestId('branding-logo-input')
    setFile(
      input,
      new File([new Uint8Array([0xff, 0xd8, 0xff, 0xd9])], 'logo.jpg', { type: 'image/jpeg' }),
    )
    await user.click(screen.getByRole('button', { name: /Save|Speichern/ }))
    await waitFor(() => {
      expect(api.patchCompanyBranding).toHaveBeenCalled()
    })
    const [, branding] = vi.mocked(api.patchCompanyBranding).mock.calls[0]!
    expect(branding.logo_b64).toBeTruthy()
    expect(branding.logo_mime).toMatch(/jpeg/)
  })

  it('preview calls generate with company_id and sets iframe src', async () => {
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <TestProviders>
          <AuthProvider>
            <BrandingPage />
          </AuthProvider>
        </TestProviders>
      </MemoryRouter>,
    )
    await screen.findByTestId('branding-preview-btn')
    await user.click(screen.getByTestId('branding-preview-btn'))
    await waitFor(() => {
      expect(api.generate).toHaveBeenCalled()
    })
    const opts = vi.mocked(api.generate).mock.calls[0]![1]
    expect(opts?.companyId).toBe(1)
    expect(opts?.format).toBe('zugferd')
    const iframe = await screen.findByTestId('branding-preview')
    expect(iframe).toHaveAttribute('src', expect.stringContaining('data:application/pdf'))
  })
})

describe('branding i18n parity', () => {
  it('every branding.* key exists in both locales', () => {
    const deKeys = Object.keys(de).filter((k) => k.startsWith('branding.') || k === 'nav.branding')
    const enKeys = Object.keys(en).filter((k) => k.startsWith('branding.') || k === 'nav.branding')
    expect(deKeys.sort()).toEqual(enKeys.sort())
    expect(deKeys.length).toBeGreaterThan(10)
  })
})
