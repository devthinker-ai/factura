import { useI18n } from '@/components/I18nProvider'
import { useT } from '@/components/I18nProvider'
import { cn } from '@/lib/utils'
import type { Lang } from '@/lib/i18n'

/** DE | EN segment control. Icon-only collapsed shows active code. */
export function LangSwitcher({ collapsed }: { collapsed?: boolean }) {
  const { lang, setLang } = useI18n()
  const t = useT()

  if (collapsed) {
    return (
      <button
        type="button"
        className="flex h-8 w-full items-center justify-center rounded-md text-[11px] font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
        aria-label={t('lang.label')}
        title={`${t('lang.label')}: ${lang.toUpperCase()}`}
        data-testid="lang-switcher"
        data-lang={lang}
        onClick={() => setLang(lang === 'de' ? 'en' : 'de')}
      >
        {lang.toUpperCase()}
      </button>
    )
  }

  const set = (next: Lang) => () => setLang(next)

  return (
    <div
      className="flex rounded-md border border-border p-0.5"
      role="group"
      aria-label={t('lang.label')}
      data-testid="lang-switcher"
      data-lang={lang}
    >
      {(['de', 'en'] as const).map((code) => (
        <button
          key={code}
          type="button"
          className={cn(
            'flex-1 rounded px-2 py-1 text-[11px] font-medium transition-colors',
            lang === code
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:text-foreground',
          )}
          aria-pressed={lang === code}
          onClick={set(code)}
        >
          {t(`lang.${code}`)}
        </button>
      ))}
    </div>
  )
}
