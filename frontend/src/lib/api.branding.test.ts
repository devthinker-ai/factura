import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '@/lib/api'

describe('api branding helpers', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const u = String(url)
        if (u === '/companies/3' && (!init || init.method === 'GET' || !init.method)) {
          return new Response(
            JSON.stringify({
              id: 3,
              name: 'Co',
              is_default: true,
              created_at: '2026-01-01T00:00:00Z',
              has_logo: true,
              branding: { logo_b64: 'abc', logo_mime: 'image/jpeg', accent: '#112233' },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
        }
        if (u === '/companies/3' && init?.method === 'PATCH') {
          const body = JSON.parse(String(init.body))
          expect(body.branding).toBeTruthy()
          expect(body.branding.logo_b64).toBe('xyz')
          return new Response(
            JSON.stringify({
              id: 3,
              name: 'Co',
              is_default: true,
              created_at: '2026-01-01T00:00:00Z',
              has_logo: true,
              branding: body.branding,
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
        }
        if (u.includes('/invoices/generate')) {
          expect(u).toContain('company_id=3')
          expect(u).toContain('format=zugferd')
          return new Response(
            JSON.stringify({
              self_check: true,
              xrechnung: '',
              zugferd_pdf_base64: 'JVBERi0x',
              filename_xrechnung: 'a.xml',
              filename_zugferd: 'a.pdf',
            }),
            { status: 201, headers: { 'Content-Type': 'application/json' } },
          )
        }
        return new Response(JSON.stringify({ error: 'unexpected ' + u }), { status: 500 })
      }),
    )
  })

  it('getCompany hits /companies/{id}', async () => {
    const c = await api.getCompany(3)
    expect(c.id).toBe(3)
    expect(c.branding?.logo_b64).toBe('abc')
  })

  it('patchCompanyBranding sends branding body', async () => {
    const c = await api.patchCompanyBranding(3, {
      logo_b64: 'xyz',
      logo_mime: 'image/jpeg',
      accent: '#1E5AA8',
    })
    expect(c.branding?.logo_b64).toBe('xyz')
  })

  it('generate appends company_id', async () => {
    const res = await api.generate({ type: 'standard' }, { format: 'zugferd', companyId: 3 })
    expect(res.self_check).toBe(true)
    expect(res.zugferd_pdf_base64).toBeTruthy()
  })
})
