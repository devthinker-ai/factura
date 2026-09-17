import { cn } from '@/lib/utils'

/** Lightweight loading placeholder — prefer over spinner soup. */
export function Skeleton({
  className,
  rows = 5,
}: {
  className?: string
  rows?: number
}) {
  return (
    <div className={cn('space-y-2', className)} data-testid="skeleton" aria-hidden>
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          className="h-9 animate-pulse rounded-md bg-muted"
          style={{ opacity: 1 - i * 0.08 }}
        />
      ))}
    </div>
  )
}
