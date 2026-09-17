import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from '@/components/ThemeProvider'
import { useT } from '@/components/I18nProvider'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { ThemePreference } from '@/lib/theme'

const icons: Record<ThemePreference, typeof Sun> = {
  light: Sun,
  dark: Moon,
  system: Monitor,
}

/** Cycles light → dark → system. Icon-only when collapsed. */
export function ThemeToggle({ collapsed }: { collapsed?: boolean }) {
  const { preference, cycle } = useTheme()
  const t = useT()
  const Icon = icons[preference]
  const modeLabel = t(`theme.${preference}`)
  const label = t('theme.toggle', { mode: modeLabel })

  return (
    <Button
      type="button"
      variant="ghost"
      size={collapsed ? 'icon' : 'sm'}
      className={cn(!collapsed && 'w-full justify-start gap-2 px-2')}
      onClick={cycle}
      aria-label={label}
      title={label}
      data-testid="theme-toggle"
      data-theme={preference}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && <span className="truncate text-xs">{modeLabel}</span>}
    </Button>
  )
}
