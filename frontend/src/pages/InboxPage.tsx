import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, ChevronUp, Download, Upload } from 'lucide-react'
import * as api from '@/lib/api'
import { ApiError, type Row } from '@/lib/api'
import { StatusBadge } from '@/components/StatusBadge'
import { EmptyState, EmptyStateCta } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import {
  OnboardingPanel,
  isOnboardingDismissed,
} from '@/components/OnboardingPanel'
import { useT } from '@/components/I18nProvider'
import { useTableKeyboardNav } from '@/hooks/useTableKeyboardNav'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/sonner'
import { cn, downloadBlob, formatMoney, truncate } from '@/lib/utils'

const LIMIT = 50

export function InboxPage() {
  const t = useT()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [debouncedQ, setDebouncedQ] = useState('')
  const [status, setStatus] = useState('')
  const [vendor, setVendor] = useState('')
  const [since, setSince] = useState('')
  const [until, setUntil] = useState('')
  const [offset, setOffset] = useState(0)
  const [rows, setRows] = useState<Row[]>([])
  const [total, setTotal] = useState(0)
  const [vendors, setVendors] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(true)
  const [uploads, setUploads] = useState<{ name: string; progress: string }[]>([])
  const [showOnboarding, setShowOnboarding] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQ(q), 300)
    return () => window.clearTimeout(timer)
  }, [q])

  const filters = useMemo(
    () => ({
      q: debouncedQ || undefined,
      status: status || undefined,
      vendor: vendor || undefined,
      since: since || undefined,
      until: until || undefined,
    }),
    [debouncedQ, status, vendor, since, until],
  )

  const load = useCallback(
    async (off: number, append: boolean) => {
      setLoading(true)
      setLoadError(false)
      try {
        const res = await api.list({ ...filters, limit: LIMIT, offset: off })
        setTotal(res.total)
        setRows((prev) => (append ? [...prev, ...res.rows] : res.rows))
        setOffset(off)
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) {
          /* TokenBar handles */
        } else {
          setLoadError(true)
          toast({ title: t('inbox.error'), variant: 'destructive' })
        }
      } finally {
        setLoading(false)
      }
    },
    [filters, t],
  )

  useEffect(() => {
    void load(0, false)
  }, [load])

  useEffect(() => {
    api
      .vendors(200)
      .then(setVendors)
      .catch(() => {})
  }, [])

  // First-run: empty archive + Peppol not configured + not dismissed
  useEffect(() => {
    if (loading || loadError || total > 0 || isOnboardingDismissed()) {
      setShowOnboarding(false)
      return
    }
    let cancelled = false
    Promise.all([api.health(), api.peppolStatus()])
      .then(([, st]) => {
        if (cancelled) return
        setShowOnboarding(!st.ap_configured)
      })
      .catch(() => {
        if (!cancelled) setShowOnboarding(true)
      })
    return () => {
      cancelled = true
    }
  }, [loading, loadError, total])

  const { onKeyDown } = useTableKeyboardNav({
    rowCount: rows.length,
    onOpen: (index) => {
      const r = rows[index]
      if (r) navigate(`/invoices/${r.id}`)
    },
  })

  const onFiles = async (files: FileList | File[]) => {
    const list = [...files]
    if (!list.length) return
    setUploads(list.map((f) => ({ name: f.name, progress: t('inbox.upload.progress') })))
    let needRefetch = false
    for (let i = 0; i < list.length; i++) {
      const file = list[i]
      try {
        const buf =
          typeof file.arrayBuffer === 'function'
            ? await file.arrayBuffer()
            : await new Promise<ArrayBuffer>((resolve, reject) => {
                const r = new FileReader()
                r.onload = () => resolve(r.result as ArrayBuffer)
                r.onerror = () => reject(r.error ?? new Error('read failed'))
                r.readAsArrayBuffer(file)
              })
        const res = await api.ingest(buf, { source: 'web-upload' })
        setUploads((u) =>
          u.map((x, j) =>
            j === i ? { ...x, progress: t('inbox.upload.ok', { id: res.id }) } : x,
          ),
        )
        needRefetch = true
        toast({
          title: t('inbox.toast.ingested', { name: file.name }),
          description: t('inbox.toast.ingested_desc', { id: res.id, status: res.status }),
          variant: 'success',
        })
      } catch (e) {
        if (e instanceof ApiError) {
          const body = e.body as { id?: number; detail?: string; error?: string }
          if (e.status === 409) {
            const id = body?.id
            setUploads((u) =>
              u.map((x, j) =>
                j === i ? { ...x, progress: t('inbox.upload.duplicate') } : x,
              ),
            )
            toast({
              title: t('inbox.toast.duplicate', { id: id ?? '?' }),
              variant: 'warning',
            })
          } else if (e.status === 415) {
            const id = body?.id
            setUploads((u) =>
              u.map((x, j) =>
                j === i ? { ...x, progress: t('inbox.upload.parse_error') } : x,
              ),
            )
            toast({
              title: t('inbox.toast.unrecognized'),
              description: (
                <span>
                  kept as parse_error (id{' '}
                  {id != null ? (
                    <a className="underline" href={`/app/invoices/${id}`}>
                      {id}
                    </a>
                  ) : (
                    '?'
                  )}
                  )
                </span>
              ),
              variant: 'warning',
            })
            needRefetch = true
          } else {
            setUploads((u) =>
              u.map((x, j) =>
                j === i
                  ? { ...x, progress: t('inbox.upload.error_status', { status: e.status }) }
                  : x,
              ),
            )
            toast({
              title: t('inbox.toast.upload_failed', { status: e.status }),
              description: body?.error || body?.detail,
              variant: 'destructive',
            })
          }
        } else {
          setUploads((u) =>
            u.map((x, j) => (j === i ? { ...x, progress: t('inbox.upload.error') } : x)),
          )
        }
      }
    }
    if (needRefetch) void load(0, false)
  }

  const onExport = async () => {
    try {
      const blob = await api.exportCsv(filters)
      downloadBlob(blob, `factura-export.csv`)
    } catch {
      toast({ title: t('inbox.export_failed'), variant: 'destructive' })
    }
  }

  const showing = rows.length

  return (
    <div className="flex flex-col">
      <div className="sticky top-0 z-10 space-y-3 border-b border-border bg-background/95 px-4 py-3 backdrop-blur">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-[160px] flex-1">
            <Label htmlFor="q">{t('inbox.search')}</Label>
            <Input
              id="q"
              placeholder={t('inbox.search_placeholder')}
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <div className="w-36">
            <Label htmlFor="status">{t('inbox.status')}</Label>
            <Select
              id="status"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              aria-label={t('inbox.status')}
            >
              <option value="">{t('inbox.status_all')}</option>
              <option value="valid">valid</option>
              <option value="invalid">invalid</option>
              <option value="parse_error">parse_error</option>
            </Select>
          </div>
          <div className="w-44">
            <Label htmlFor="vendor">{t('inbox.vendor')}</Label>
            <Select
              id="vendor"
              value={vendor}
              onChange={(e) => setVendor(e.target.value)}
              aria-label={t('inbox.vendor')}
            >
              <option value="">{t('inbox.vendor_all')}</option>
              {vendors.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </Select>
          </div>
          <div className="w-36">
            <Label htmlFor="since">{t('inbox.since')}</Label>
            <Input
              id="since"
              type="date"
              value={since}
              onChange={(e) => setSince(e.target.value)}
            />
          </div>
          <div className="w-36">
            <Label htmlFor="until">{t('inbox.until')}</Label>
            <Input
              id="until"
              type="date"
              value={until}
              onChange={(e) => setUntil(e.target.value)}
            />
          </div>
          <Button variant="outline" onClick={() => void onExport()}>
            <Download className="h-3.5 w-3.5" />
            {t('inbox.export')}
          </Button>
        </div>

        <div className="rounded-md border border-dashed border-border">
          <button
            type="button"
            className="flex w-full items-center justify-between px-3 py-2 text-left text-sm"
            onClick={() => setUploadOpen((o) => !o)}
          >
            <span className="flex items-center gap-2 text-muted-foreground">
              <Upload className="h-3.5 w-3.5" />
              {t('inbox.upload')}
            </span>
            {uploadOpen ? (
              <ChevronUp className="h-4 w-4 text-muted-foreground" />
            ) : (
              <ChevronDown className="h-4 w-4 text-muted-foreground" />
            )}
          </button>
          {uploadOpen && (
            <div
              className="border-t border-border px-3 py-4"
              onDragOver={(e) => {
                e.preventDefault()
                e.stopPropagation()
              }}
              onDrop={(e) => {
                e.preventDefault()
                const list = e.dataTransfer?.files
                if (!list?.length) return
                void onFiles(Array.from(list as unknown as File[]))
              }}
              data-testid="inbox-dropzone"
            >
              <div
                className={cn(
                  'flex cursor-pointer flex-col items-center justify-center rounded-md border border-dashed border-border bg-muted/30 px-4 py-6 text-center text-sm text-muted-foreground hover:bg-muted/50',
                )}
                onClick={() => fileRef.current?.click()}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') fileRef.current?.click()
                }}
              >
                <p>{t('inbox.drop')}</p>
                <p className="mt-1 text-xs">{t('inbox.drop_hint')}</p>
              </div>
              <input
                ref={fileRef}
                type="file"
                multiple
                className="hidden"
                data-testid="inbox-upload"
                onChange={(e) => {
                  const files = e.target.files
                  if (files && files.length > 0) {
                    void onFiles(Array.from(files))
                  }
                  e.target.value = ''
                }}
              />
              {uploads.length > 0 && (
                <ul className="mt-2 space-y-1 text-xs">
                  {uploads.map((u) => (
                    <li key={u.name} className="flex justify-between gap-2">
                      <span className="truncate">{u.name}</span>
                      <span className="shrink-0 text-muted-foreground">{u.progress}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="px-4 py-3">
        <div className="mb-2 flex items-baseline justify-between gap-2">
          <h1 className="text-base font-semibold tracking-tight">
            {t('inbox.title', { count: total, showing })}
          </h1>
        </div>

        {showOnboarding && (
          <div className="mb-4">
            <OnboardingPanel onDismiss={() => setShowOnboarding(false)} />
          </div>
        )}

        {loadError ? (
          <ErrorState message={t('inbox.error')} onRetry={() => void load(0, false)} />
        ) : loading && rows.length === 0 ? (
          <Skeleton rows={6} />
        ) : rows.length === 0 ? (
          <EmptyState
            title={t('inbox.empty.title')}
            body={t('inbox.empty.body')}
            action={
              <>
                <EmptyStateCta to="/new">{t('inbox.empty.cta_new')}</EmptyStateCta>
                <EmptyStateCta to="/settings/peppol" variant="outline">
                  {t('inbox.empty.cta_settings')}
                </EmptyStateCta>
              </>
            }
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('inbox.col.date')}</TableHead>
                <TableHead>{t('inbox.col.vendor')}</TableHead>
                <TableHead>{t('inbox.col.number')}</TableHead>
                <TableHead>{t('inbox.col.status')}</TableHead>
                <TableHead>{t('inbox.col.format')}</TableHead>
                <TableHead className="text-right">{t('inbox.col.total')}</TableHead>
                <TableHead className="text-right">{t('inbox.col.vat')}</TableHead>
                <TableHead>{t('inbox.col.source')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r, index) => (
                <TableRow
                  key={r.id}
                  className="cursor-pointer"
                  tabIndex={0}
                  data-row-index={index}
                  data-testid={`inbox-row-${r.id}`}
                  onClick={() => navigate(`/invoices/${r.id}`)}
                  onKeyDown={(e) => onKeyDown(e, index)}
                >
                  <TableCell className="tabular-nums text-muted-foreground">
                    {r.invoice_date || r.created_at?.slice(0, 10) || '–'}
                  </TableCell>
                  <TableCell className="max-w-[140px] truncate font-medium">
                    {r.vendor_name || '–'}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{r.invoice_number || '–'}</TableCell>
                  <TableCell>
                    <StatusBadge status={r.status} />
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{r.format || '–'}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatMoney(r.total, r.currency || 'EUR')}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-muted-foreground">
                    {formatMoney(r.vat_amount, r.currency || 'EUR')}
                  </TableCell>
                  <TableCell
                    className="max-w-[100px] truncate text-xs text-muted-foreground"
                    title={r.source || undefined}
                  >
                    {r.source ? truncate(r.source, 24) : '–'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        {showing < total && (
          <div className="mt-4 flex justify-center">
            <Button
              variant="outline"
              onClick={() => void load(offset + LIMIT, true)}
              disabled={loading}
            >
              {t('inbox.load_older')}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
