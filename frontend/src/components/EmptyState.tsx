import { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'

export function EmptyState({
  title,
  body,
  action,
  className,
}: {
  title: string
  body?: string
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-3 px-4 py-14 text-center',
        className,
      )}
      data-testid="empty-state"
    >
      <h2 className="text-sm font-semibold tracking-tight">{title}</h2>
      {body && <p className="max-w-md text-sm text-muted-foreground">{body}</p>}
      {action && <div className="mt-1 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  )
}

const ctaBase =
  'inline-flex h-7 items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-2.5 text-xs font-medium transition-[color,background-color,transform] duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

export function EmptyStateCta({
  to,
  children,
  variant = 'default',
}: {
  to: string
  children: ReactNode
  variant?: 'default' | 'outline'
}) {
  return (
    <Link
      to={to}
      className={cn(
        ctaBase,
        variant === 'outline'
          ? 'border border-border bg-transparent hover:bg-secondary'
          : 'border border-border bg-primary text-primary-foreground shadow-sm hover:bg-primary/90',
      )}
    >
      {children}
    </Link>
  )
}
