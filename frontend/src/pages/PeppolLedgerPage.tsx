import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight } from 'lucide-react'
import * as api from '@/lib/api'
import type { PeppolMessage } from '@/lib/api'
import { StatusBadge } from '@/components/StatusBadge'
import { EmptyState, EmptyStateCta } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { useT } from '@/components/I18nProvider'
import { useTableKeyboardNav } from '@/hooks/useTableKeyboardNav'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

function Evidence({ id }: { id: number }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [msg, setMsg] = useState<PeppolMessage | null>(null)
  const [err, setErr] = useState(false)

  useEffect(() => {
    if (!open || msg) return
    api
      .peppolMessage(id)
      .then(setMsg)
      .catch(() => setErr(true))
  }, [open, id, msg])

  return (
    <div className="mt-1">
      <button
        type="button"
        className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        onClick={() => setOpen((o) => !o)}
      >
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        {t('peppol.evidence')}
      </button>
      {open && (
        <div className="mt-2 space-y-2 rounded-md border border-border bg-muted/30 p-2 text-xs">
          {err && <p className="text-destructive">{t('peppol.evidence_fail')}</p>}
          {!msg && !err && <p className="text-muted-foreground">{t('common.loading')}</p>}
          {msg && (
            <>
              <div>
                <p className="mb-1 font-medium">{t('peppol.bis')}</p>
                <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px]">
                  {msg.bis_xml || t('peppol.empty_xml')}
                </pre>
              </div>
              <div>
                <p className="mb-1 font-medium">{t('peppol.receipt')}</p>
                <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px]">
                  {msg.receipt || t('peppol.empty_xml')}
                </pre>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function MessageTable({ rows }: { rows: PeppolMessage[] }) {
  const t = useT()
  const { onKeyDown } = useTableKeyboardNav({
    rowCount: rows.length,
    onOpen: () => {
      /* ledger rows expand evidence via click; Enter focuses evidence affordance */
    },
  })

  if (rows.length === 0) {
    return (
      <EmptyState
        title={t('peppol.empty.title')}
        body={t('peppol.empty.body')}
        action={
          <EmptyStateCta to="/settings/peppol">{t('peppol.empty.cta')}</EmptyStateCta>
        }
      />
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('peppol.col.time')}</TableHead>
          <TableHead>{t('peppol.col.route')}</TableHead>
          <TableHead>{t('peppol.col.status')}</TableHead>
          <TableHead>{t('peppol.col.invoice')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((m, index) => (
          <TableRow
            key={m.id}
            tabIndex={0}
            data-row-index={index}
            onKeyDown={(e) => onKeyDown(e, index)}
          >
            <TableCell className="align-top text-xs tabular-nums text-muted-foreground">
              {m.created_at}
              <Evidence id={m.id} />
            </TableCell>
            <TableCell className="align-top font-mono text-xs">
              {m.from_peppol_id || '–'} → {m.to_peppol_id || '–'}
            </TableCell>
            <TableCell className="align-top">
              <StatusBadge status={m.status} />
            </TableCell>
            <TableCell className="align-top text-sm">
              {m.invoice_id ? (
                <Link to={`/invoices/${m.invoice_id}`} className="underline hover:text-foreground">
                  #{m.invoice_id}
                </Link>
              ) : (
                '–'
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export function PeppolLedgerPage() {
  const t = useT()
  const [inbound, setInbound] = useState<PeppolMessage[]>([])
  const [outbound, setOutbound] = useState<PeppolMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [planLimit, setPlanLimit] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError(false)
    setPlanLimit(false)
    Promise.all([
      api.peppolMessages({ direction: 'in', limit: 200 }),
      api.peppolMessages({ direction: 'out', limit: 200 }),
    ])
      .then(([inn, out]) => {
        setInbound(inn)
        setOutbound(out)
      })
      .catch((e) => {
        if (api.isPlanLimit(e)) {
          setPlanLimit(true)
          return
        }
        setError(true)
      })
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  return (
    <div className="space-y-4 p-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">{t('peppol.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('peppol.subtitle')}</p>
        </div>
        <Button variant="outline" size="sm" onClick={load}>
          {t('common.refresh')}
        </Button>
      </header>

      {planLimit && (
        <EmptyState
          title={t('peppol.plan_limit.title')}
          body={t('peppol.plan_limit.body')}
          action={<EmptyStateCta to="/settings/license">{t('peppol.plan_limit.cta')}</EmptyStateCta>}
        />
      )}
      {error && !planLimit && (
        <ErrorState message={t('peppol.load_failed')} onRetry={load} />
      )}
      {!error && !planLimit && loading ? (
        <Skeleton rows={5} />
      ) : !error && !planLimit ? (
        <Tabs defaultValue="inbound">
          <TabsList>
            <TabsTrigger value="inbound">
              {t('peppol.inbound', { count: inbound.length })}
            </TabsTrigger>
            <TabsTrigger value="outbound">
              {t('peppol.outbound', { count: outbound.length })}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="inbound">
            <MessageTable rows={inbound} />
          </TabsContent>
          <TabsContent value="outbound">
            <MessageTable rows={outbound} />
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  )
}
