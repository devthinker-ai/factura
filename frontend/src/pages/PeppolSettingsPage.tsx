import { useCallback, useEffect, useRef, useState } from 'react'
import * as api from '@/lib/api'
import type { PeppolParticipant, PeppolStatus } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { EmptyState, EmptyStateCta } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/sonner'

export function PeppolSettingsPage() {
  const t = useT()
  const auth = useAuth()
  const [status, setStatus] = useState<PeppolStatus | null>(null)
  const [participants, setParticipants] = useState<PeppolParticipant[]>([])
  const [selfId, setSelfId] = useState('')
  const [saving, setSaving] = useState(false)
  const [pinging, setPinging] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [planLimit, setPlanLimit] = useState(false)
  const selfInputRef = useRef<HTMLInputElement>(null)

  const load = useCallback(() => {
    setLoading(true)
    setLoadError(false)
    setPlanLimit(false)
    Promise.all([api.peppolStatus(), api.peppolParticipants()])
      .then(([st, ps]) => {
        setStatus(st)
        setSelfId(st.self_id || '')
        setParticipants(ps)
      })
      .catch((e) => {
        if (api.isPlanLimit(e)) {
          setPlanLimit(true)
          return
        }
        setLoadError(true)
        toast({ title: t('settings.load_failed'), variant: 'destructive' })
      })
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const saveSelf = async () => {
    const id = selfId.trim()
    if (!id || !id.includes(':')) {
      toast({ title: t('settings.id_format'), variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      await api.peppolAddParticipant({
        peppol_id: id,
        name: t('settings.self_name'),
        service_type: 'buyer-seller',
        is_self: true,
      })
      toast({ title: t('settings.self_saved'), variant: 'success' })
      load()
    } catch {
      toast({ title: t('settings.save_failed'), variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  const testConnection = async () => {
    setPinging(true)
    try {
      const res = await api.peppolPing()
      if (res.ok) {
        toast({
          title: t('settings.connected', { name: res.ap_name || 'AP' }),
          variant: 'success',
        })
      } else {
        toast({ title: res.error || t('settings.ping_failed'), variant: 'destructive' })
      }
    } catch (e) {
      const msg = e instanceof api.ApiError ? String(e.message) : t('settings.ping_failed')
      toast({ title: msg, variant: 'destructive' })
    } finally {
      setPinging(false)
    }
  }

  if (planLimit) {
    return (
      <div className="mx-auto max-w-2xl p-4">
        <EmptyState
          title={t('peppol.plan_limit.title')}
          body={t('peppol.plan_limit.body')}
          action={<EmptyStateCta to="/settings/license">{t('peppol.plan_limit.cta')}</EmptyStateCta>}
        />
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="mx-auto max-w-2xl p-4">
        <ErrorState message={t('settings.load_failed')} onRetry={load} />
      </div>
    )
  }

  if (loading && !status) {
    return (
      <div className="mx-auto max-w-2xl space-y-4 p-4">
        <Skeleton rows={6} />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-4 p-4">
      <header>
        <h1 className="text-lg font-semibold tracking-tight">{t('settings.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('settings.subtitle')}</p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.ap')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-muted-foreground">{t('settings.ap_label')}</span>
            <span className="font-mono">{status?.ap_name || '–'}</span>
            {status?.ap_configured ? (
              <Badge variant="success">{t('settings.configured')}</Badge>
            ) : (
              <Badge variant="warning">{t('settings.not_configured')}</Badge>
            )}
          </div>
          <div>
            <span className="text-muted-foreground">{t('settings.callback')} </span>
            <span className="font-mono text-xs">
              {status?.callback_url || t('settings.callback_unset')}
            </span>
          </div>
          <div>
            <span className="text-muted-foreground">{t('settings.poll')} </span>
            {status?.poll_enabled ? (
              <span>
                {t('settings.poll_on', { interval: status.poll_interval || '60s' })}
              </span>
            ) : (
              <span>{t('settings.poll_off')}</span>
            )}
          </div>
          {auth.canWrite && (
          <Button
            size="sm"
            variant="outline"
            disabled={pinging}
            onClick={() => void testConnection()}
            data-testid="peppol-ping"
          >
            {pinging ? t('settings.testing') : t('settings.test')}
          </Button>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.self')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {auth.canWrite && (
          <>
          <div className="space-y-1.5">
            <Label htmlFor="self-id">{t('settings.peppol_id')}</Label>
            <Input
              ref={selfInputRef}
              id="self-id"
              value={selfId}
              onChange={(e) => setSelfId(e.target.value)}
              placeholder="DE:DE123456789"
              className="font-mono"
            />
          </div>
          <Button size="sm" disabled={saving} onClick={() => void saveSelf()}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
          </>
          )}
          {!auth.canWrite && (
            <p className="font-mono text-sm">{status?.self_id || '—'}</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('settings.participants')}</CardTitle>
        </CardHeader>
        <CardContent>
          {participants.length === 0 ? (
            <EmptyState
              className="py-8"
              title={t('settings.participants_empty.title')}
              body={t('settings.participants_empty.body')}
              action={
                auth.canWrite ? (
                <Button
                  size="sm"
                  onClick={() => selfInputRef.current?.focus()}
                >
                  {t('settings.participants_empty.cta')}
                </Button>
                ) : undefined
              }
            />
          ) : (
            <ul className="space-y-1 text-sm">
              {participants.map((p) => (
                <li key={p.peppol_id} className="flex flex-wrap gap-2 font-mono text-xs">
                  <span>{p.peppol_id}</span>
                  {p.is_self && <Badge variant="info">{t('settings.self_badge')}</Badge>}
                  <span className="text-muted-foreground">{p.name}</span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
