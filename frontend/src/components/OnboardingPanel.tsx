import { Link } from 'react-router-dom'
import { useT } from '@/components/I18nProvider'
import { Button } from '@/components/ui/button'

export const ONBOARDING_KEY = 'factura:onboarding-dismissed'

export function isOnboardingDismissed(
  storage: Pick<Storage, 'getItem'> | null = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): boolean {
  try {
    return storage?.getItem(ONBOARDING_KEY) === '1'
  } catch {
    return false
  }
}

export function dismissOnboarding(
  storage: Pick<Storage, 'setItem'> | null = typeof localStorage !== 'undefined'
    ? localStorage
    : null,
): void {
  try {
    storage?.setItem(ONBOARDING_KEY, '1')
  } catch {
    /* ignore */
  }
}

/** First-run panel when archive is empty and Peppol is not configured. */
export function OnboardingPanel({ onDismiss }: { onDismiss: () => void }) {
  const t = useT()
  const steps = [
    { title: t('onboarding.step1.title'), body: t('onboarding.step1.body') },
    { title: t('onboarding.step2.title'), body: t('onboarding.step2.body') },
    { title: t('onboarding.step3.title'), body: t('onboarding.step3.body') },
  ]

  return (
    <div
      className="rounded-md border border-border bg-card p-4"
      data-testid="onboarding-panel"
    >
      <h2 className="text-sm font-semibold tracking-tight">{t('onboarding.title')}</h2>
      <ol className="mt-3 space-y-3">
        {steps.map((s, i) => (
          <li key={s.title} className="flex gap-3 text-sm">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium tabular-nums">
              {i + 1}
            </span>
            <div>
              <p className="font-medium">{s.title}</p>
              <p className="mt-0.5 text-muted-foreground">{s.body}</p>
            </div>
          </li>
        ))}
      </ol>
      <div className="mt-4 flex flex-wrap gap-2">
        <Link
          to="/settings/peppol"
          className="inline-flex h-7 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-secondary"
        >
          {t('onboarding.cta_settings')}
        </Link>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            dismissOnboarding()
            onDismiss()
          }}
        >
          {t('onboarding.dismiss')}
        </Button>
      </div>
    </div>
  )
}
