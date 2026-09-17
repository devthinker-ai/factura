import { useCallback, useEffect, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import {
  ChevronLeft,
  ChevronRight,
  FilePlus2,
  Inbox,
  KeyRound,
  LogOut,
  Network,
  Palette,
  Settings,
  Shield,
  Users,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { health } from '@/lib/api'
import { TokenBar } from '@/components/TokenBar'
import { ThemeToggle } from '@/components/ThemeToggle'
import { LangSwitcher } from '@/components/LangSwitcher'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

export function AppShell() {
  const t = useT()
  const auth = useAuth()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)
  const [version, setVersion] = useState('…')
  const [retryKey, setRetryKey] = useState(0)

  const loadHealth = useCallback(() => {
    health()
      .then((h) => setVersion(h.version || 'dev'))
      .catch(() => setVersion('offline'))
  }, [])

  useEffect(() => {
    loadHealth()
  }, [loadHealth, retryKey])

  // Users mode without session → login (except while AuthProvider still loading).
  useEffect(() => {
    if (auth.loading) return
    if (auth.mode === 'users' && !auth.user) {
      navigate('/login', { replace: true })
    }
  }, [auth.loading, auth.mode, auth.user, navigate])

  const navItems: {
    to: string
    labelKey: string
    icon: typeof Inbox
    end?: boolean
    show?: boolean
  }[] = [
    { to: '/', labelKey: 'nav.inbox', icon: Inbox, end: true, show: true },
    { to: '/new', labelKey: 'nav.new', icon: FilePlus2, show: !auth.isViewer },
    { to: '/peppol', labelKey: 'nav.peppol', icon: Network, show: true },
    {
      to: '/settings/license',
      labelKey: 'nav.license',
      icon: KeyRound,
      show: true,
    },
    { to: '/settings/peppol', labelKey: 'nav.settings', icon: Settings, show: true },
    { to: '/settings/branding', labelKey: 'nav.branding', icon: Palette, show: true },
    {
      to: '/settings/security',
      labelKey: 'nav.security',
      icon: Shield,
      show: auth.mode === 'users',
    },
    {
      to: '/settings/users',
      labelKey: 'nav.users',
      icon: Users,
      // Visible in open mode too so the first admin can bootstrap from the UI.
      show: auth.canAdmin,
    },
  ]

  return (
    <div className="flex h-svh flex-col overflow-hidden">
      <TokenBar onRetry={() => setRetryKey((k) => k + 1)} />
      {auth.user && (
        <div
          className="flex items-center justify-end gap-2 border-b border-border bg-card px-3 py-1.5 text-sm"
          data-testid="user-header"
        >
          <span className="text-muted-foreground">{auth.user.name || auth.user.email}</span>
          <Badge variant="secondary" data-testid="role-chip">
            {auth.user.role}
          </Badge>
          <Button
            variant="ghost"
            size="sm"
            onClick={auth.logout}
            data-testid="logout"
            aria-label={t('auth.logout')}
          >
            <LogOut className="mr-1 h-3.5 w-3.5" />
            {t('auth.logout')}
          </Button>
        </div>
      )}
      <div className="flex min-h-0 flex-1">
        <aside
          className={cn(
            'sticky top-0 flex h-full shrink-0 flex-col border-r border-border bg-card transition-[width] duration-200',
            collapsed ? 'w-14' : 'w-52',
          )}
        >
          <div className="flex h-12 items-center justify-between gap-2 border-b border-border px-3">
            {!collapsed && (
              <span className="truncate text-sm font-semibold tracking-tight">Factura</span>
            )}
            <Button
              variant="ghost"
              size="icon"
              aria-label={collapsed ? t('nav.expand') : t('nav.collapse')}
              onClick={() => setCollapsed((c) => !c)}
            >
              {collapsed ? (
                <ChevronRight className="h-4 w-4" />
              ) : (
                <ChevronLeft className="h-4 w-4" />
              )}
            </Button>
          </div>
          <nav className="flex flex-1 flex-col gap-0.5 p-2">
            {navItems
              .filter((item) => item.show !== false)
              .map((item) => {
                const label = t(item.labelKey)
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
                    className={({ isActive }) =>
                      cn(
                        'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
                        isActive
                          ? 'bg-accent text-accent-foreground font-medium'
                          : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                        collapsed && 'justify-center px-0',
                      )
                    }
                    title={label}
                  >
                    <item.icon className="h-4 w-4 shrink-0" />
                    {!collapsed && <span>{label}</span>}
                  </NavLink>
                )
              })}
          </nav>
          <div
            className={cn(
              'flex flex-col gap-1.5 border-t border-border p-2',
              collapsed && 'items-center',
            )}
          >
            <ThemeToggle collapsed={collapsed} />
            <LangSwitcher collapsed={collapsed} />
            <div className="px-1 pt-0.5 text-[11px] text-muted-foreground">
              {!collapsed ? <span>v{version}</span> : <span title={version}>v</span>}
            </div>
          </div>
        </aside>
        <main className="min-w-0 flex-1 overflow-auto">
          <Outlet key={retryKey} />
        </main>
      </div>
    </div>
  )
}
