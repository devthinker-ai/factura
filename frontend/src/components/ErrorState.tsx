import { Button } from '@/components/ui/button'
import { useT } from '@/components/I18nProvider'
import { cn } from '@/lib/utils'

/** Recoverable fetch error — same Retry pattern as TokenBar. */
export function ErrorState({
  message,
  onRetry,
  className,
}: {
  message: string
  onRetry?: () => void
  className?: string
}) {
  const t = useT()
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-3 rounded-md border border-destructive/30 bg-destructive/5 px-4 py-10 text-center',
        className,
      )}
      role="alert"
      data-testid="error-state"
    >
      <p className="text-sm text-destructive">{message}</p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t('common.retry')}
        </Button>
      )}
    </div>
  )
}
