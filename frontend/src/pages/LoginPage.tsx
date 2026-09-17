import { FormEvent, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import * as api from '@/lib/api'
import { ApiError } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { LangSwitcher } from '@/components/LangSwitcher'
import { ThemeToggle } from '@/components/ThemeToggle'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function LoginPage() {
  const t = useT()
  const auth = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = params.get('next') || '/'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState(false)
  const [retryAfter, setRetryAfter] = useState(0)
  const codeRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (auth.mode === 'open' || auth.mode === 'token') {
      navigate(next, { replace: true })
    }
    if (auth.mode === 'users' && auth.user) {
      navigate(next, { replace: true })
    }
  }, [auth.mode, auth.user, navigate, next])

  useEffect(() => {
    if (mfaToken) codeRef.current?.focus()
  }, [mfaToken])

  useEffect(() => {
    if (retryAfter <= 0) return
    const id = window.setInterval(() => {
      setRetryAfter((s) => Math.max(0, s - 1))
    }, 1000)
    return () => window.clearInterval(id)
  }, [retryAfter])

  const finish = (result: api.LoginResult) => {
    if (result.token && result.user) {
      auth.setUserFromLogin(result.user, result.token)
      navigate(next, { replace: true })
    }
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const res = await api.login(email.trim(), password)
      if (res.mfa_required && res.mfa_token) {
        setMfaToken(res.mfa_token)
        setCode('')
        return
      }
      finish(res)
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const ra = Number(err.headers.get('Retry-After') || '60')
        setRetryAfter(ra)
        setError(err.message)
      } else if (err instanceof ApiError) {
        setError(err.message) // verbatim API English
      } else {
        setError(String(err))
      }
    } finally {
      setBusy(false)
    }
  }

  const submitMFA = async (value: string) => {
    if (!mfaToken) return
    setError('')
    setBusy(true)
    try {
      const res = await api.loginMFA(mfaToken, value)
      finish(res)
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const ra = Number(err.headers.get('Retry-After') || '60')
        setRetryAfter(ra)
        setError(err.message)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError(String(err))
      }
    } finally {
      setBusy(false)
    }
  }

  const onMFASubmit = async (e: FormEvent) => {
    e.preventDefault()
    await submitMFA(code.trim())
  }

  return (
    <div className="flex min-h-svh flex-col bg-background">
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <span className="text-sm font-semibold tracking-tight">Factura</span>
        <div className="flex items-center gap-2">
          <ThemeToggle collapsed={false} />
          <LangSwitcher collapsed={false} />
        </div>
      </div>
      <div className="flex flex-1 items-center justify-center p-6">
        <Card className="w-full max-w-sm" data-testid="login-card">
          <CardHeader>
            <CardTitle>{mfaToken ? t('login.mfa_title') : t('login.title')}</CardTitle>
          </CardHeader>
          <CardContent>
            {!mfaToken ? (
              <form onSubmit={onSubmit} className="space-y-3">
                <div className="space-y-1.5">
                  <Label htmlFor="login-email">{t('login.email')}</Label>
                  <Input
                    id="login-email"
                    type="email"
                    autoComplete="username"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="login-password">{t('login.password')}</Label>
                  <Input
                    id="login-password"
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                </div>
                {error && (
                  <p className="text-sm text-destructive" data-testid="login-error" role="alert">
                    {error}
                  </p>
                )}
                {retryAfter > 0 && (
                  <p className="text-sm text-muted-foreground" data-testid="login-retry">
                    {t('login.retry_after', { seconds: retryAfter })}
                  </p>
                )}
                <Button type="submit" className="w-full" disabled={busy || retryAfter > 0}>
                  {busy ? t('login.signing_in') : t('login.submit')}
                </Button>
              </form>
            ) : (
              <form onSubmit={onMFASubmit} className="space-y-3">
                <p className="text-sm text-muted-foreground">{t('login.mfa_hint')}</p>
                {!recovery ? (
                  <div className="space-y-1.5">
                    <Label htmlFor="login-code">{t('login.code')}</Label>
                    <Input
                      id="login-code"
                      ref={codeRef}
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      maxLength={6}
                      value={code}
                      onChange={(e) => {
                        const v = e.target.value.replace(/\D/g, '').slice(0, 6)
                        setCode(v)
                        if (v.length === 6) void submitMFA(v)
                      }}
                      data-testid="mfa-code"
                    />
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    <Label htmlFor="login-recovery">{t('login.recovery')}</Label>
                    <Input
                      id="login-recovery"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      placeholder="xxxx-xxxx"
                      data-testid="mfa-recovery"
                    />
                  </div>
                )}
                <button
                  type="button"
                  className="text-xs text-muted-foreground underline"
                  onClick={() => {
                    setRecovery((r) => !r)
                    setCode('')
                  }}
                >
                  {recovery ? t('login.use_totp') : t('login.use_recovery')}
                </button>
                {error && (
                  <p className="text-sm text-destructive" data-testid="login-error" role="alert">
                    {error}
                  </p>
                )}
                {retryAfter > 0 && (
                  <p className="text-sm text-muted-foreground" data-testid="login-retry">
                    {t('login.retry_after', { seconds: retryAfter })}
                  </p>
                )}
                <Button type="submit" className="w-full" disabled={busy || retryAfter > 0}>
                  {busy ? t('login.signing_in') : t('login.verify')}
                </Button>
              </form>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
