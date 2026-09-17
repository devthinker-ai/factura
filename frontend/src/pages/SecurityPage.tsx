import { FormEvent, useEffect, useState } from 'react'
import * as api from '@/lib/api'
import { ApiError } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { ErrorState } from '@/components/ErrorState'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/sonner'

export function SecurityPage() {
  const t = useT()
  const auth = useAuth()
  const [setup, setSetup] = useState<{
    secret: string
    provisioning_uri: string
    qr: string
  } | null>(null)
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState<string[] | null>(null)
  const [removeOpen, setRemoveOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const enrolled = !!auth.user?.totp_enabled

  useEffect(() => {
    // Refresh once on mount so totp_enabled is current after enroll elsewhere.
    void auth.refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount only
  }, [])

  const startSetup = async () => {
    setBusy(true)
    setError('')
    setRecovery(null)
    try {
      const res = await api.setup2FA()
      setSetup(res)
      setCode('')
    } catch (e) {
      toast({
        title: e instanceof Error ? e.message : t('security.setup_failed'),
        variant: 'destructive',
      })
    } finally {
      setBusy(false)
    }
  }

  const confirm = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await api.confirm2FA(code.trim())
      setRecovery(res.recovery_codes)
      setSetup(null)
      await auth.refresh()
      toast({ title: t('security.enabled'), variant: 'success' })
    } catch (err) {
      if (err instanceof ApiError) setError(err.message)
      else setError(String(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async () => {
    setBusy(true)
    try {
      await api.delete2FA()
      setRemoveOpen(false)
      setRecovery(null)
      await auth.refresh()
      toast({ title: t('security.removed'), variant: 'success' })
    } catch (e) {
      toast({
        title: e instanceof Error ? e.message : t('security.remove_failed'),
        variant: 'destructive',
      })
    } finally {
      setBusy(false)
    }
  }

  const copyAll = async () => {
    if (!recovery) return
    try {
      await navigator.clipboard.writeText(recovery.join('\n'))
      toast({ title: t('common.copied'), variant: 'success' })
    } catch {
      toast({ title: t('common.copy_failed'), variant: 'destructive' })
    }
  }

  if (auth.mode === 'users' && !auth.user) {
    return <ErrorState message={t('security.need_login')} onRetry={() => void auth.refresh()} />
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{t('security.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('security.subtitle')}</p>
      </div>

      <Card data-testid="2fa-card">
        <CardHeader>
          <CardTitle>{t('security.2fa')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {enrolled ? (
            <>
              <p className="text-sm">
                {t('security.enabled_since', {
                  at: auth.user?.totp_confirmed_at || '—',
                })}
              </p>
              <Button
                variant="destructive"
                onClick={() => setRemoveOpen(true)}
                data-testid="2fa-remove"
              >
                {t('security.remove')}
              </Button>
            </>
          ) : (
            <Button onClick={startSetup} disabled={busy} data-testid="2fa-setup">
              {t('security.setup')}
            </Button>
          )}

          {setup && (
            <Dialog
              open
              onClose={() => setSetup(null)}
              title={t('security.setup')}
              description={t('security.scan_hint')}
            >
              <div className="space-y-3" data-testid="2fa-qr-dialog">
                <img src={setup.qr} alt="QR" className="mx-auto h-48 w-48" />
                <p className="break-all font-mono text-xs text-muted-foreground">
                  {setup.provisioning_uri}
                </p>
                <div className="space-y-1">
                  <Label>{t('security.secret')}</Label>
                  <code className="block rounded bg-muted px-2 py-1 font-mono text-xs">
                    {setup.secret}
                  </code>
                </div>
                <form onSubmit={confirm} className="space-y-2">
                  <Label htmlFor="confirm-code">{t('login.code')}</Label>
                  <Input
                    id="confirm-code"
                    inputMode="numeric"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                    data-testid="2fa-confirm-code"
                  />
                  {error && (
                    <p className="text-sm text-destructive" role="alert">
                      {error}
                    </p>
                  )}
                  <Button type="submit" disabled={busy || code.length !== 6}>
                    {t('security.confirm')}
                  </Button>
                </form>
              </div>
            </Dialog>
          )}

          {recovery && recovery.length > 0 && (
            <div
              className="rounded-md border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/40"
              data-testid="recovery-block"
            >
              <p className="mb-2 text-sm font-medium">{t('security.recovery_once')}</p>
              <ul className="mb-2 space-y-0.5 font-mono text-sm">
                {recovery.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
              <Button size="sm" variant="outline" onClick={copyAll}>
                {t('security.copy_all')}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={removeOpen}
        onClose={() => setRemoveOpen(false)}
        title={t('security.remove')}
        description={t('security.remove_hint')}
      >
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setRemoveOpen(false)}>
            {t('common.dismiss')}
          </Button>
          <Button variant="destructive" onClick={remove} disabled={busy}>
            {t('security.remove')}
          </Button>
        </div>
      </Dialog>
    </div>
  )
}
