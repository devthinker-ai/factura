import { FormEvent, useEffect, useState } from 'react'
import { onUnauthorized, setToken } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { X } from 'lucide-react'

export function TokenBar({ onRetry }: { onRetry?: () => void }) {
  const t = useT()
  const auth = useAuth()
  const [visible, setVisible] = useState(false)
  const [value, setValue] = useState('')
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    return onUnauthorized(() => {
      // Users mode → login page (AuthProvider); TokenBar is token-mode only.
      if (auth.mode === 'users') return
      if (!dismissed) setVisible(true)
    })
  }, [dismissed, auth.mode])

  if (auth.mode === 'users' || !visible || dismissed) return null

  const save = (e: FormEvent) => {
    e.preventDefault()
    const tok = value.trim()
    if (!tok) return
    setToken(tok)
    setVisible(false)
    onRetry?.()
  }

  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 border-b border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100"
      data-testid="token-bar"
    >
      <span className="shrink-0">{t('token.prompt')}</span>
      <form onSubmit={save} className="flex flex-1 items-center gap-2">
        <Input
          type="password"
          autoComplete="off"
          placeholder="FACTURA_TOKEN"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="h-7 max-w-xs bg-background"
          aria-label={t('token.aria')}
        />
        <Button type="submit" size="sm">
          {t('token.save')}
        </Button>
      </form>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t('token.dismiss')}
        onClick={() => {
          setDismissed(true)
          setVisible(false)
        }}
      >
        <X className="h-4 w-4" />
      </Button>
    </div>
  )
}
