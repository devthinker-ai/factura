import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ChevronDown, ChevronUp, Copy, Download, FileText, Network } from 'lucide-react'
import * as api from '@/lib/api'
import {
  ApiError,
  type PeppolParticipant,
  type PeppolSendResult,
  type Report,
  type Row,
  type Violation,
} from '@/lib/api'
import { StatusBadge } from '@/components/StatusBadge'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog } from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/sonner'
import { downloadBlob, formatMoney, truncate } from '@/lib/utils'

function parseReport(row: Row): Report | null {
  const raw = row.report_json
  if (!raw) return null
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as Report
    } catch {
      return null
    }
  }
  return raw as Report
}

function parseDoc(row: Row): unknown {
  const raw = row.doc_gobl_json
  if (raw == null) return null
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw)
    } catch {
      return raw
    }
  }
  return raw
}

export function InvoiceDetailPage() {
  const t = useT()
  const auth = useAuth()
  const { id: idParam } = useParams()
  const id = Number(idParam)
  const [row, setRow] = useState<Row | null>(null)
  const [error, setError] = useState<'404' | 'other' | null>(null)
  const [jsonOpen, setJsonOpen] = useState(false)
  const [sendOpen, setSendOpen] = useState(false)
  const [toId, setToId] = useState('')
  const [participants, setParticipants] = useState<PeppolParticipant[]>([])
  const [sending, setSending] = useState(false)
  const [sendResult, setSendResult] = useState<PeppolSendResult | null>(null)
  const [sendError, setSendError] = useState<{ status: number; detail: string } | null>(null)
  const [showBackTop, setShowBackTop] = useState(false)
  const [evidence, setEvidence] = useState<api.EvidenceResponse | null>(null)
  const [audit, setAudit] = useState<api.AuditResponse | null>(null)
  const [auditBusy, setAuditBusy] = useState(false)
  const reportRef = useRef<HTMLDivElement>(null)

  const reload = () => {
    if (!Number.isFinite(id)) {
      setError('404')
      return
    }
    setError(null)
    setRow(null)
    api
      .get(id)
      .then(setRow)
      .catch((e) => {
        if (e instanceof ApiError && e.status === 404) setError('404')
        else setError('other')
      })
  }

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  useEffect(() => {
    if (!Number.isFinite(id)) return
    setEvidence(null)
    api
      .evidence(id)
      .then(setEvidence)
      .catch(() => setEvidence(null))
  }, [id])

  useEffect(() => {
    const onScroll = () => {
      const el = reportRef.current
      if (!el) return
      setShowBackTop(window.scrollY > el.offsetTop + 320)
    }
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [row])

  const report = useMemo(() => (row ? parseReport(row) : null), [row])
  const doc = useMemo(() => (row ? parseDoc(row) : null), [row])
  const formattedJson = useMemo(() => {
    if (doc == null) return ''
    try {
      return JSON.stringify(doc, null, 2)
    } catch {
      return String(doc)
    }
  }, [doc])

  if (error === '404') {
    return (
      <div className="p-6">
        <p className="text-sm">
          {t('detail.not_found', { id: idParam ?? '' })}{' '}
          <Link to="/" className="underline">
            {t('detail.back_inbox')}
          </Link>
        </p>
      </div>
    )
  }

  if (error === 'other') {
    return (
      <div className="p-6">
        <ErrorState message={t('detail.load_failed')} onRetry={reload} />
      </div>
    )
  }

  if (!row) {
    return (
      <div className="p-6">
        <Skeleton rows={8} />
      </div>
    )
  }

  const isPdf = (row.format || '').toLowerCase().includes('pdf')
  const violations: Violation[] = report?.violations ?? []
  const isParseError = row.status === 'parse_error'

  const parseErrorReason = (): string => {
    if (report?.summary) return report.summary
    const v = report?.violations?.[0]
    if (v?.human) return v.human
    return t('detail.parse_fallback')
  }

  const downloadOriginal = async () => {
    try {
      const { blob, filename } = await api.fetchOriginalBlob(row.id)
      downloadBlob(blob, filename)
    } catch {
      toast({ title: t('detail.download_failed'), variant: 'destructive' })
    }
  }

  const openPdf = async () => {
    try {
      const { blob } = await api.fetchOriginalBlob(row.id)
      const url = URL.createObjectURL(blob)
      window.open(url, '_blank', 'noopener,noreferrer')
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch {
      toast({ title: t('detail.open_pdf_failed'), variant: 'destructive' })
    }
  }

  const copyExternal = async () => {
    try {
      await navigator.clipboard.writeText(row.external_id || '')
      toast({ title: t('detail.external_copied'), variant: 'success' })
    } catch {
      toast({ title: t('common.copy_failed'), variant: 'destructive' })
    }
  }

  const copyJson = async () => {
    try {
      await navigator.clipboard.writeText(formattedJson)
      toast({ title: t('detail.json_copied'), variant: 'success' })
    } catch {
      toast({ title: t('common.copy_failed'), variant: 'destructive' })
    }
  }

  const copyRule = async (ruleId: string) => {
    try {
      await navigator.clipboard.writeText(ruleId)
      toast({ title: t('detail.rule_copied'), variant: 'success' })
    } catch {
      toast({ title: t('common.copy_failed'), variant: 'destructive' })
    }
  }

  const openSend = async () => {
    setSendResult(null)
    setSendError(null)
    setSendOpen(true)
    try {
      const ps = await api.peppolParticipants()
      setParticipants(ps.filter((p) => !p.is_self))
    } catch {
      setParticipants([])
    }
  }

  const doSend = async () => {
    const to = toId.trim()
    if (!to) {
      toast({ title: t('detail.send.enter_id'), variant: 'destructive' })
      return
    }
    setSending(true)
    setSendResult(null)
    setSendError(null)
    try {
      const res = await api.peppolSend({ invoice_id: row!.id, to })
      setSendResult(res)
      toast({ title: t('detail.send.ok'), variant: 'success' })
    } catch (e) {
      if (e instanceof ApiError) {
        const body = e.body as { detail?: string; error?: string } | null
        setSendError({
          status: e.status,
          detail: body?.detail || body?.error || e.message,
        })
      } else {
        setSendError({ status: 0, detail: t('detail.send.failed') })
      }
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="space-y-4 p-4">
      <nav className="text-xs text-muted-foreground">
        <Link to="/" className="hover:text-foreground hover:underline">
          {t('detail.breadcrumb_inbox')}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground">
          {t('detail.breadcrumb_invoice', { id: row.id })}
        </span>
      </nav>

      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold tracking-tight">
              {row.vendor_name || t('detail.unknown_vendor')}
            </h1>
            <StatusBadge status={row.status} />
            {row.invoice_type ? (
              <Badge variant="outline" data-testid="invoice-type">
                {t('detail.invoice_type', { code: row.invoice_type })}
              </Badge>
            ) : null}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            <span className="font-mono">{row.invoice_number || '–'}</span>
            <span className="mx-2">·</span>
            <span className="tabular-nums">{row.invoice_date || '–'}</span>
          </p>
          <p className="mt-2 text-xl font-semibold tabular-nums tracking-tight">
            {formatMoney(row.total, row.currency || 'EUR')}
            <span className="ml-2 text-sm font-normal text-muted-foreground">
              {t('detail.vat_label', {
                amount: formatMoney(row.vat_amount, row.currency || 'EUR'),
              })}
            </span>
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('detail.external_id')}{' '}
            <span className="font-mono">{row.external_id || '–'}</span>
            {row.source ? (
              <>
                <span className="mx-1.5">·</span>
                {t('detail.source', { source: row.source })}
              </>
            ) : null}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => void downloadOriginal()}>
            <Download className="h-3.5 w-3.5" />
            {t('detail.download_original')}
          </Button>
          {isPdf && (
            <Button variant="outline" size="sm" onClick={() => void openPdf()}>
              <FileText className="h-3.5 w-3.5" />
              {t('detail.open_pdf')}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => void copyExternal()}>
            <Copy className="h-3.5 w-3.5" />
            {t('detail.copy_external')}
          </Button>
          {row.status === 'valid' && auth.canWrite && (
            <Button size="sm" onClick={() => void openSend()} data-testid="send-peppol">
              <Network className="h-3.5 w-3.5" />
              {t('detail.send_peppol')}
            </Button>
          )}
        </div>
      </header>

      {(evidence || audit) && (
        <Card data-testid="gobd-evidence">
          <CardHeader
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={auditBusy}
                onClick={() => {
                  setAuditBusy(true)
                  api
                    .audit()
                    .then(setAudit)
                    .catch(() =>
                      toast({ title: t('detail.evidence.verify_failed'), variant: 'destructive' }),
                    )
                    .finally(() => setAuditBusy(false))
                }}
              >
                {t('detail.evidence.verify')}
              </Button>
            }
          >
            <CardTitle>{t('detail.evidence.title')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {evidence && (
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={evidence.verified ? 'success' : 'destructive'}>
                  {evidence.verified
                    ? t('detail.evidence.verified')
                    : t('detail.evidence.unverified')}
                </Badge>
                <span className="font-mono text-xs break-all">{evidence.hash}</span>
              </div>
            )}
            {audit && (
              <p className="text-xs text-muted-foreground" data-testid="audit-summary">
                {audit.ok
                  ? t('detail.evidence.audit_ok', { n: audit.link_count, head: audit.head })
                  : t('detail.evidence.audit_broken', {
                      id: audit.first_broken_id,
                      n: audit.link_count,
                    })}
              </p>
            )}
          </CardContent>
        </Card>
      )}

      <Dialog
        open={sendOpen}
        onClose={() => setSendOpen(false)}
        title={t('detail.send.title')}
        className="max-w-md"
      >
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="peppol-to">{t('detail.send.to')}</Label>
            <Input
              id="peppol-to"
              list="peppol-participants"
              className="font-mono"
              placeholder={t('detail.send.placeholder')}
              value={toId}
              onChange={(e) => setToId(e.target.value)}
            />
            <datalist id="peppol-participants">
              {participants.map((p) => (
                <option key={p.peppol_id} value={p.peppol_id}>
                  {p.name || p.peppol_id}
                </option>
              ))}
            </datalist>
          </div>
          <Button disabled={sending} onClick={() => void doSend()}>
            {sending ? t('detail.send.sending') : t('detail.send.send')}
          </Button>
          {sendResult && (
            <div
              className="rounded-md border border-border bg-muted/40 p-3 text-sm"
              data-testid="peppol-send-ok"
            >
              <p className="font-medium">{t('detail.send.delivered')}</p>
              <p className="mt-1 text-muted-foreground">
                {t('detail.send.message_status', {
                  id: sendResult.message_id,
                  status: sendResult.status,
                })}
              </p>
              {sendResult.receipt && (
                <pre className="mt-2 overflow-auto font-mono text-[11px]">
                  {JSON.stringify(sendResult.receipt, null, 2)}
                </pre>
              )}
              <Link to="/peppol" className="mt-2 inline-block text-xs underline">
                {t('detail.send.open_ledger')}
              </Link>
            </div>
          )}
          {sendError && (
            <div
              className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm"
              data-testid="peppol-send-err"
            >
              <p className="font-medium">
                {sendError.status === 424
                  ? t('detail.send.ap_rejected')
                  : t('detail.send.error', { status: sendError.status || '' })}
              </p>
              <p className="mt-1 text-muted-foreground">{sendError.detail}</p>
            </div>
          )}
        </div>
      </Dialog>

      <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
        <div ref={reportRef}>
        <Card>
          <CardHeader className="sticky top-0 z-[1] bg-card/95 backdrop-blur">
            <CardTitle>{t('detail.report_title')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {isParseError ? (
              <div className="rounded-md border border-border bg-muted/40 p-3 text-sm">
                <p className="font-medium">{t('detail.parse_error')}</p>
                <p className="mt-1 text-muted-foreground">{parseErrorReason()}</p>
              </div>
            ) : (
              <>
                <p className="text-sm">
                  {report?.summary ||
                    (row.status === 'valid' ? t('detail.passed') : t('detail.validation_done'))}
                </p>
                {violations.length === 0 ? (
                  <p className="text-sm text-emerald-700 dark:text-emerald-400">
                    {t('detail.passed')}
                  </p>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('detail.col.rule')}</TableHead>
                        <TableHead>{t('detail.col.message')}</TableHead>
                        <TableHead>{t('detail.col.value')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {violations.map((v, i) => (
                        <TableRow key={`${v.rule_id}-${i}`}>
                          <TableCell className="font-mono text-xs">
                            <button
                              type="button"
                              className="rounded px-0.5 hover:bg-muted"
                              title={t('detail.rule_copied')}
                              onClick={() => void copyRule(v.rule_id)}
                              data-testid={`rule-id-${v.rule_id}`}
                            >
                              {v.rule_id}
                            </button>
                          </TableCell>
                          {/* Verbatim invariant: engine `human` — never wrap in t() */}
                          <TableCell className="text-sm">{v.human}</TableCell>
                          <TableCell
                            className="max-w-[120px] truncate font-mono text-xs text-muted-foreground"
                            title={v.value}
                          >
                            {v.value ? truncate(v.value, 60) : '–'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </>
            )}
          </CardContent>
        </Card>
        </div>

        <Card>
          <CardHeader
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => void copyJson()}
                disabled={!formattedJson}
              >
                <Copy className="h-3.5 w-3.5" />
                {t('detail.copy_json')}
              </Button>
            }
          >
            <CardTitle>{t('detail.doc_title')}</CardTitle>
          </CardHeader>
          <CardContent>
            {!formattedJson ? (
              <p className="text-sm text-muted-foreground">{t('detail.no_gobl')}</p>
            ) : (
              <>
                <button
                  type="button"
                  className="mb-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                  onClick={() => setJsonOpen((o) => !o)}
                >
                  {jsonOpen ? (
                    <ChevronUp className="h-3.5 w-3.5" />
                  ) : (
                    <ChevronDown className="h-3.5 w-3.5" />
                  )}
                  {jsonOpen ? t('detail.hide_json') : t('detail.show_json')}
                  <Badge variant="secondary" className="ml-1">
                    SoR
                  </Badge>
                </button>
                {jsonOpen && (
                  <pre className="max-h-[480px] overflow-auto rounded-md bg-muted/50 p-3 font-mono text-[11px] leading-relaxed">
                    {formattedJson}
                  </pre>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {showBackTop && violations.length > 0 && (
        <button
          type="button"
          className="fixed bottom-6 right-6 z-20 rounded-md border border-border bg-card px-3 py-1.5 text-xs shadow-md hover:bg-muted"
          onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
        >
          {t('common.back_to_top')}
        </button>
      )}
    </div>
  )
}
