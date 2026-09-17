import { useCallback, useEffect, useState } from 'react'
import { Copy, KeyRound, Star, Trash2 } from 'lucide-react'
import * as api from '@/lib/api'
import type { APIKeyRow, Company, LicenseInfo } from '@/lib/api'
import { ApiError, isPlanLimit, planLimitDetail } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { EmptyState } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/sonner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function LicensePage() {
  const t = useT()
  const auth = useAuth()
  const [lic, setLic] = useState<LicenseInfo | null>(null)
  const [companies, setCompanies] = useState<Company[]>([])
  const [keys, setKeys] = useState<APIKeyRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [pasteKey, setPasteKey] = useState('')
  const [savingKey, setSavingKey] = useState(false)
  const [coName, setCoName] = useState('')
  const [coVat, setCoVat] = useState('')
  const [coPeppol, setCoPeppol] = useState('')
  const [coBanner, setCoBanner] = useState('')
  const [keyName, setKeyName] = useState('')
  const [keyRpm, setKeyRpm] = useState('')
  const [keyMonthly, setKeyMonthly] = useState('')
  const [plaintextOnce, setPlaintextOnce] = useState<string | null>(null)
  const [keyBanner, setKeyBanner] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    setError(false)
    Promise.all([api.getLicense(), api.listCompanies(), api.listAPIKeys()])
      .then(([l, c, k]) => {
        setLic(l)
        setCompanies(c)
        setKeys(k)
      })
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const onPasteLicense = async () => {
    setSavingKey(true)
    try {
      const next = await api.postLicense(pasteKey.trim())
      setLic(next)
      setPasteKey('')
      toast({ title: t('license.toast.applied'), variant: 'success' })
    } catch (e) {
      const detail =
        e instanceof ApiError && typeof e.body === 'object' && e.body && 'detail' in e.body
          ? String((e.body as { detail: string }).detail)
          : e instanceof Error
            ? e.message
            : 'error'
      toast({ title: t('license.toast.invalid'), description: detail, variant: 'destructive' })
    } finally {
      setSavingKey(false)
    }
  }

  const onRemoveLicense = async () => {
    try {
      await api.deleteLicense()
      toast({ title: t('license.toast.removed'), variant: 'success' })
      load()
    } catch (e) {
      toast({
        title: t('license.toast.remove_failed'),
        description: e instanceof Error ? e.message : undefined,
        variant: 'destructive',
      })
    }
  }

  const onAddCompany = async () => {
    setCoBanner('')
    try {
      await api.addCompany({
        name: coName.trim(),
        seller_vat_id: coVat.trim() || undefined,
        peppol_id: coPeppol.trim() || undefined,
      })
      setCoName('')
      setCoVat('')
      setCoPeppol('')
      load()
    } catch (e) {
      if (isPlanLimit(e)) {
        setCoBanner(planLimitDetail(e) || t('license.plan_limit'))
        return
      }
      toast({
        title: t('license.companies.add_failed'),
        description: e instanceof Error ? e.message : undefined,
        variant: 'destructive',
      })
    }
  }

  const onCreateKey = async () => {
    setKeyBanner('')
    setPlaintextOnce(null)
    try {
      const created = await api.createAPIKey({
        name: keyName.trim(),
        rpm: keyRpm ? Number(keyRpm) : undefined,
        monthly_invoices: keyMonthly ? Number(keyMonthly) : undefined,
      })
      setPlaintextOnce(created.key)
      setKeyName('')
      setKeyRpm('')
      setKeyMonthly('')
      load()
    } catch (e) {
      if (isPlanLimit(e)) {
        setKeyBanner(planLimitDetail(e) || t('license.plan_limit'))
        return
      }
      toast({
        title: t('license.keys.create_failed'),
        description: e instanceof Error ? e.message : undefined,
        variant: 'destructive',
      })
    }
  }

  if (error) {
    return (
      <div className="p-4">
        <ErrorState message={t('license.load_failed')} onRetry={load} />
      </div>
    )
  }

  if (loading || !lic) {
    return (
      <div className="space-y-4 p-4">
        <Skeleton rows={4} />
      </div>
    )
  }

  const trial = !lic.licensed
  const planLabel = trial ? t('license.plan.trial') : lic.plan

  return (
    <div className="space-y-4 p-4">
      <header>
        <h1 className="text-lg font-semibold tracking-tight">{t('license.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('license.subtitle')}</p>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle className="text-base">{t('license.plan.card')}</CardTitle>
          <Badge variant={trial ? 'secondary' : 'default'}>{planLabel}</Badge>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">{t('license.plan.subject')}</dt>
              <dd>{lic.subject || t('common.empty_dash')}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('license.plan.expires')}</dt>
              <dd>{lic.expires ? new Date(lic.expires).toLocaleDateString() : t('common.empty_dash')}</dd>
            </div>
          </dl>
          <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            <CapStat
              label={t('license.caps.companies')}
              value={`${lic.in_use.companies}/${lic.caps.max_companies}`}
            />
            <CapStat
              label={t('license.caps.api_keys')}
              value={`${lic.in_use.api_keys}/${lic.caps.max_api_keys}`}
            />
            <CapStat
              label={t('license.caps.invoices')}
              value={String(lic.in_use.invoices_this_month)}
            />
            <CapStat
              label={t('license.caps.peppol')}
              value={lic.caps.peppol ? '✓' : '—'}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t('license.plan.hint')}</p>
          {auth.canAdmin && (
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              value={pasteKey}
              onChange={(e) => setPasteKey(e.target.value)}
              placeholder={t('license.plan.paste_placeholder')}
              aria-label={t('license.plan.paste_placeholder')}
              className="font-mono text-xs"
            />
            <Button onClick={onPasteLicense} disabled={!pasteKey.trim() || savingKey}>
              {savingKey ? t('common.saving') : t('license.plan.apply')}
            </Button>
            {lic.licensed && (
              <Button variant="outline" onClick={onRemoveLicense}>
                {t('license.plan.remove')}
              </Button>
            )}
          </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('license.companies.card')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {coBanner && (
            <div
              className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm"
              role="status"
              data-testid="plan-limit-banner"
            >
              {coBanner}
            </div>
          )}
          {companies.length === 0 ? (
            <EmptyState title={t('license.companies.empty')} body={t('license.companies.empty_body')} />
          ) : (
            <ul className="divide-y divide-border rounded-md border border-border">
              {companies.map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-2 px-3 py-2 text-sm">
                  <div>
                    <p className="font-medium">
                      {c.name}
                      {c.is_default && (
                        <Star className="ml-1 inline h-3.5 w-3.5 fill-current text-amber-500" />
                      )}
                    </p>
                    <p className="font-mono text-xs text-muted-foreground">
                      {c.seller_vat_id || t('common.empty_dash')}
                    </p>
                  </div>
                  {!c.is_default && auth.canWrite && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() =>
                        api.setDefaultCompany(c.id).then(load).catch(() =>
                          toast({ title: t('license.companies.add_failed'), variant: 'destructive' }),
                        )
                      }
                    >
                      {t('license.companies.set_default')}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
          {auth.canWrite && (
          <>
          <div className="grid gap-2 sm:grid-cols-3">
            <div>
              <Label htmlFor="co-name">{t('license.companies.name')}</Label>
              <Input id="co-name" value={coName} onChange={(e) => setCoName(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="co-vat">{t('license.companies.vat')}</Label>
              <Input id="co-vat" value={coVat} onChange={(e) => setCoVat(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="co-peppol">{t('license.companies.peppol')}</Label>
              <Input id="co-peppol" value={coPeppol} onChange={(e) => setCoPeppol(e.target.value)} />
            </div>
          </div>
          <Button size="sm" onClick={onAddCompany} disabled={!coName.trim()}>
            {t('license.companies.add')}
          </Button>
          </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('license.keys.card')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {keyBanner && (
            <div
              className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm"
              role="status"
            >
              {keyBanner}
            </div>
          )}
          {plaintextOnce && (
            <div
              className="space-y-2 rounded-md border border-primary/40 bg-primary/5 p-3"
              data-testid="plaintext-key"
            >
              <p className="text-xs font-medium text-amber-700 dark:text-amber-400">
                {t('license.keys.shown_once')}
              </p>
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all font-mono text-xs">{plaintextOnce}</code>
                <Button
                  size="icon"
                  variant="outline"
                  aria-label={t('common.copied')}
                  onClick={() => {
                    void navigator.clipboard.writeText(plaintextOnce).then(
                      () => toast({ title: t('common.copied'), variant: 'success' }),
                      () => toast({ title: t('common.copy_failed'), variant: 'destructive' }),
                    )
                  }}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
          {keys.length === 0 ? (
            <EmptyState title={t('license.keys.empty')} body={t('license.keys.empty_body')} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('license.keys.col.id')}</TableHead>
                  <TableHead>{t('license.keys.col.name')}</TableHead>
                  <TableHead>{t('license.keys.col.rpm')}</TableHead>
                  <TableHead>{t('license.keys.col.usage')}</TableHead>
                  <TableHead>{t('license.keys.col.enabled')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((k) => {
                  const pct =
                    k.monthly_invoices > 0
                      ? Math.min(100, Math.round((k.invoices_this_month / k.monthly_invoices) * 100))
                      : 0
                  return (
                    <TableRow key={k.id}>
                      <TableCell className="max-w-[10rem] truncate font-mono text-[11px]" title={k.id}>
                        {k.id}
                      </TableCell>
                      <TableCell>{k.name}</TableCell>
                      <TableCell className="tabular-nums">{k.rpm}</TableCell>
                      <TableCell>
                        <div className="space-y-1">
                          <span className="text-xs tabular-nums">
                            {k.invoices_this_month}/{k.monthly_invoices}
                          </span>
                          <div className="h-1.5 w-24 overflow-hidden rounded bg-muted">
                            <div className="h-full bg-primary transition-all" style={{ width: `${pct}%` }} />
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        {auth.canAdmin ? (
                        <input
                          type="checkbox"
                          checked={k.enabled}
                          aria-label={t('license.keys.col.enabled')}
                          onChange={(e) => {
                            api
                              .patchAPIKey(k.id, e.target.checked)
                              .then(load)
                              .catch(() =>
                                toast({ title: t('license.keys.create_failed'), variant: 'destructive' }),
                              )
                          }}
                        />
                        ) : (
                          <span>{k.enabled ? '✓' : '—'}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {auth.canAdmin && (
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label={t('license.keys.delete')}
                          onClick={() => {
                            if (!window.confirm(t('license.keys.delete_confirm'))) return
                            api
                              .deleteAPIKey(k.id)
                              .then(load)
                              .catch(() =>
                                toast({ title: t('license.keys.create_failed'), variant: 'destructive' }),
                              )
                          }}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
          {auth.canAdmin && (
          <>
          <div className="grid gap-2 sm:grid-cols-3">
            <div>
              <Label htmlFor="key-name">{t('license.keys.name')}</Label>
              <Input id="key-name" value={keyName} onChange={(e) => setKeyName(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="key-rpm">{t('license.keys.rpm')}</Label>
              <Input
                id="key-rpm"
                value={keyRpm}
                onChange={(e) => setKeyRpm(e.target.value)}
                placeholder="30"
                inputMode="numeric"
              />
            </div>
            <div>
              <Label htmlFor="key-monthly">{t('license.keys.monthly')}</Label>
              <Input
                id="key-monthly"
                value={keyMonthly}
                onChange={(e) => setKeyMonthly(e.target.value)}
                placeholder="1000"
                inputMode="numeric"
              />
            </div>
          </div>
          <Button size="sm" onClick={onCreateKey} disabled={!keyName.trim()}>
            <KeyRound className="mr-1 h-3.5 w-3.5" />
            {t('license.keys.create')}
          </Button>
          </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function CapStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className="text-sm font-semibold tabular-nums">{value}</p>
    </div>
  )
}
