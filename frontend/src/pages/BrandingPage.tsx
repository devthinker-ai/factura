import { useCallback, useEffect, useRef, useState } from 'react'
import * as api from '@/lib/api'
import type { Company, CompanyBranding } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { EmptyState } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/sonner'
import { defaultFormState, toGobl } from '@/lib/gobl'

const MAX_LOGO_BYTES = 512_000

type Draft = {
  accent: string
  headerText: string
  footerText: string
  logoB64: string
  logoMime: string
  logoPreview: string
}

const emptyDraft = (): Draft => ({
  accent: '',
  headerText: '',
  footerText: '',
  logoB64: '',
  logoMime: '',
  logoPreview: '',
})

function draftFromBranding(b?: CompanyBranding): Draft {
  const d = emptyDraft()
  if (!b) return d
  d.accent = b.accent || ''
  d.headerText = b.header_text || ''
  d.footerText = b.footer_text || ''
  if (b.logo_b64) {
    d.logoB64 = b.logo_b64
    d.logoMime = b.logo_mime || 'image/jpeg'
    d.logoPreview = `data:${d.logoMime};base64,${b.logo_b64}`
  }
  return d
}

export function BrandingPage() {
  const t = useT()
  const auth = useAuth()
  const fileRef = useRef<HTMLInputElement>(null)
  const [companies, setCompanies] = useState<Company[]>([])
  const [companyId, setCompanyId] = useState<number | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft())
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [saving, setSaving] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [previewSrc, setPreviewSrc] = useState('')
  const [selfCheck, setSelfCheck] = useState<boolean | null>(null)
  const [previewError, setPreviewError] = useState('')
  const [newName, setNewName] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    setLoadError(false)
    api
      .listCompanies()
      .then(async (list) => {
        setCompanies(list)
        const def = list.find((c) => c.is_default) ?? list[0]
        if (!def) {
          setCompanyId(null)
          setDraft(emptyDraft())
          return
        }
        setCompanyId(def.id)
        const full = await api.getCompany(def.id)
        setDraft(draftFromBranding(full.branding))
      })
      .catch(() => {
        setLoadError(true)
        toast({ title: t('branding.load_failed'), variant: 'destructive' })
      })
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const selectCompany = async (id: number) => {
    setCompanyId(id)
    setPreviewSrc('')
    setSelfCheck(null)
    setPreviewError('')
    try {
      const full = await api.getCompany(id)
      setDraft(draftFromBranding(full.branding))
    } catch {
      toast({ title: t('branding.load_failed'), variant: 'destructive' })
    }
  }

  const onFile = async (file: File | undefined) => {
    if (!file) return
    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      toast({ title: t('branding.logo_mime'), variant: 'destructive' })
      return
    }
    if (file.size > MAX_LOGO_BYTES) {
      toast({ title: t('branding.logo_size'), variant: 'destructive' })
      return
    }
    const buf = await file.arrayBuffer()
    const bytes = new Uint8Array(buf)
    let binary = ''
    for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]!)
    const b64 = btoa(binary)
    setDraft((d) => ({
      ...d,
      logoB64: b64,
      logoMime: file.type,
      logoPreview: `data:${file.type};base64,${b64}`,
    }))
  }

  const removeLogo = () => {
    setDraft((d) => ({ ...d, logoB64: '', logoMime: '', logoPreview: '' }))
    if (fileRef.current) fileRef.current.value = ''
  }

  const save = async () => {
    if (companyId == null) return
    setSaving(true)
    try {
      const branding: CompanyBranding = {
        accent: draft.accent || undefined,
        header_text: draft.headerText || undefined,
        footer_text: draft.footerText || undefined,
      }
      if (draft.logoB64) {
        branding.logo_b64 = draft.logoB64
        branding.logo_mime = draft.logoMime || 'image/jpeg'
      }
      const updated = await api.patchCompanyBranding(companyId, branding)
      toast({ title: t('branding.saved'), variant: 'success' })
      setDraft(draftFromBranding(updated.branding))
      const list = await api.listCompanies()
      setCompanies(list)
    } catch (e) {
      const msg = e instanceof api.ApiError ? String(e.message) : t('branding.save_failed')
      toast({ title: msg, variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  const preview = async () => {
    if (companyId == null) return
    setPreviewing(true)
    setPreviewError('')
    setSelfCheck(null)
    try {
      // Persist draft first so generate picks up branding via company_id.
      const branding: CompanyBranding = {
        accent: draft.accent || undefined,
        header_text: draft.headerText || undefined,
        footer_text: draft.footerText || undefined,
      }
      if (draft.logoB64) {
        branding.logo_b64 = draft.logoB64
        branding.logo_mime = draft.logoMime || 'image/jpeg'
      }
      if (auth.canWrite) {
        await api.patchCompanyBranding(companyId, branding)
      }
      const state = defaultFormState()
      state.code = 'PREVIEW-1'
      state.paymentTermsNotes = '14 Tage netto'
      state.supplier.name = 'Provide One GmbH'
      state.supplier.street = 'Dietmar-Hopp-Allee'
      state.supplier.locality = 'Walldorf'
      state.supplier.code = '69190'
      state.supplier.taxId = 'DE111111125'
      state.supplier.email = 'billing@example.com'
      state.supplier.phone = '+49100200300'
      state.supplier.contactName = 'Ada Lovelace'
      state.customer.name = 'Sample Consumer'
      state.customer.street = 'Werner-Heisenberg-Allee'
      state.customer.locality = 'München'
      state.customer.code = '80939'
      state.customer.taxId = 'DE282741168'
      state.customer.email = 'customer@example.com'
      state.lines = [{ id: '1', name: 'Consulting', quantity: '1', price: '100.00', vatRate: '19' }]
      const gobl = toGobl(state)
      const res = await api.generate(gobl, { format: 'zugferd', companyId })
      setSelfCheck(!!res.self_check)
      if (res.zugferd_pdf_base64) {
        setPreviewSrc(`data:application/pdf;base64,${res.zugferd_pdf_base64}`)
      } else {
        setPreviewError(t('branding.preview_empty'))
      }
    } catch (e) {
      const detail =
        e instanceof api.ApiError
          ? String((e.body as { detail?: string; error?: string })?.detail || e.message)
          : t('branding.preview_failed')
      setPreviewError(detail)
      setPreviewSrc('')
    } finally {
      setPreviewing(false)
    }
  }

  const createCompany = async () => {
    const name = newName.trim()
    if (!name) return
    try {
      const c = await api.addCompany({ name })
      toast({ title: t('branding.company_created'), variant: 'success' })
      setNewName('')
      setCompanies([c])
      setCompanyId(c.id)
      setDraft(emptyDraft())
    } catch (e) {
      const msg = e instanceof api.ApiError ? String(e.message) : t('branding.save_failed')
      toast({ title: msg, variant: 'destructive' })
    }
  }

  if (loadError) {
    return (
      <div className="mx-auto max-w-5xl p-4">
        <ErrorState message={t('branding.load_failed')} onRetry={load} />
      </div>
    )
  }

  if (loading && companies.length === 0 && companyId == null) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4">
        <Skeleton rows={8} />
      </div>
    )
  }

  if (companies.length === 0) {
    return (
      <div className="mx-auto max-w-2xl space-y-4 p-4">
        <h1 className="text-2xl font-semibold tracking-tight">{t('branding.title')}</h1>
        <p className="text-muted-foreground">{t('branding.subtitle')}</p>
        <EmptyState
          title={t('branding.empty.title')}
          body={t('branding.empty.body')}
          action={
            auth.canWrite ? (
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder={t('branding.empty.name_placeholder')}
                  aria-label={t('branding.empty.name_placeholder')}
                />
                <Button type="button" onClick={createCompany}>
                  {t('branding.empty.cta')}
                </Button>
              </div>
            ) : undefined
          }
        />
      </div>
    )
  }

  return (
    <div className="mx-auto grid max-w-6xl gap-6 p-4 lg:grid-cols-2">
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('branding.title')}</h1>
          <p className="text-muted-foreground">{t('branding.subtitle')}</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="branding-company">{t('branding.company')}</Label>
          <select
            id="branding-company"
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            value={companyId ?? ''}
            onChange={(e) => selectCompany(Number(e.target.value))}
          >
            {companies.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
                {c.is_default ? ` (${t('branding.default')})` : ''}
              </option>
            ))}
          </select>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('branding.editor')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t('branding.logo')}</Label>
              {draft.logoPreview ? (
                <div className="flex items-center gap-3">
                  <img
                    src={draft.logoPreview}
                    alt=""
                    className="h-14 max-w-[180px] object-contain"
                  />
                  {auth.canWrite && (
                    <Button type="button" variant="outline" size="sm" onClick={removeLogo}>
                      {t('branding.logo_remove')}
                    </Button>
                  )}
                </div>
              ) : null}
              {auth.canWrite && (
                <Input
                  ref={fileRef}
                  type="file"
                  accept="image/png,image/jpeg"
                  data-testid="branding-logo-input"
                  onChange={(e) => onFile(e.target.files?.[0])}
                />
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="branding-accent">{t('branding.accent')}</Label>
              <div className="flex items-center gap-2">
                <input
                  id="branding-accent"
                  type="color"
                  disabled={!auth.canWrite}
                  value={draft.accent || '#1E5AA8'}
                  onChange={(e) => setDraft((d) => ({ ...d, accent: e.target.value.toUpperCase() }))}
                  className="h-9 w-14 cursor-pointer rounded border border-input bg-transparent"
                />
                <span className="font-mono text-sm text-muted-foreground">
                  {draft.accent || t('branding.accent_default')}
                </span>
                {auth.canWrite && draft.accent && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setDraft((d) => ({ ...d, accent: '' }))}
                  >
                    {t('branding.accent_clear')}
                  </Button>
                )}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="branding-header">{t('branding.header')}</Label>
              <Input
                id="branding-header"
                disabled={!auth.canWrite}
                value={draft.headerText}
                maxLength={500}
                onChange={(e) => setDraft((d) => ({ ...d, headerText: e.target.value }))}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="branding-footer">{t('branding.footer')}</Label>
              <textarea
                id="branding-footer"
                disabled={!auth.canWrite}
                rows={3}
                maxLength={500}
                value={draft.footerText}
                onChange={(e) => setDraft((d) => ({ ...d, footerText: e.target.value }))}
                className="flex min-h-[72px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              />
            </div>

            {auth.canWrite && (
              <Button type="button" onClick={save} disabled={saving || companyId == null}>
                {saving ? t('branding.saving') : t('branding.save')}
              </Button>
            )}
            {auth.isViewer && (
              <Badge variant="secondary">{t('branding.readonly')}</Badge>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-lg font-medium">{t('branding.preview')}</h2>
          <Button
            type="button"
            variant="outline"
            onClick={preview}
            disabled={previewing || companyId == null}
            data-testid="branding-preview-btn"
          >
            {previewing ? t('branding.previewing') : t('branding.preview_run')}
          </Button>
        </div>
        {selfCheck != null && (
          <Badge variant={selfCheck ? 'default' : 'destructive'}>
            {selfCheck ? t('branding.self_check_ok') : t('branding.self_check_fail')}
          </Badge>
        )}
        {previewError && (
          <p className="text-sm text-destructive" role="alert">
            {previewError}
          </p>
        )}
        <div className="min-h-[480px] overflow-hidden rounded-md border border-border bg-muted/30">
          {previewSrc ? (
            <iframe
              title={t('branding.preview')}
              data-testid="branding-preview"
              src={previewSrc}
              className="h-[640px] w-full"
            />
          ) : (
            <p className="p-6 text-sm text-muted-foreground">{t('branding.preview_hint')}</p>
          )}
        </div>
      </div>
    </div>
  )
}
