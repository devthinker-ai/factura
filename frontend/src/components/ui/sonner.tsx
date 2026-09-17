import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from 'react'
import { cn } from '@/lib/utils'
import { X } from 'lucide-react'

export type ToastVariant = 'default' | 'success' | 'warning' | 'destructive'

export interface ToastInput {
  title: string
  description?: ReactNode
  variant?: ToastVariant
  durationMs?: number
}

interface ToastItem extends ToastInput {
  id: number
}

interface ToastContextValue {
  toast: (input: ToastInput) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

let toastFn: ((input: ToastInput) => void) | null = null

/** Imperative toast — works outside React tree once Toaster is mounted. */
export function toast(input: ToastInput) {
  if (toastFn) toastFn(input)
  else console.warn('toast:', input.title, input.description)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (input: ToastInput) => {
      const id = Date.now() + Math.random()
      const item: ToastItem = { ...input, id }
      setItems((prev) => [...prev, item])
      const ms = input.durationMs ?? 4500
      window.setTimeout(() => dismiss(id), ms)
    },
    [dismiss],
  )

  toastFn = push

  const value = useMemo(() => ({ toast: push }), [push])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <ToasterHost items={items} onDismiss={dismiss} />
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast requires ToastProvider')
  return ctx
}

function ToasterHost({
  items,
  onDismiss,
}: {
  items: ToastItem[]
  onDismiss: (id: number) => void
}) {
  return (
    <div
      className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-full max-w-sm flex-col gap-2"
      aria-live="polite"
    >
      {items.map((t) => (
        <div
          key={t.id}
          className={cn(
            'toast-enter pointer-events-auto rounded-md border bg-card p-3 shadow-lg',
            t.variant === 'success' && 'border-emerald-200 dark:border-emerald-900',
            t.variant === 'warning' && 'border-amber-200 dark:border-amber-900',
            t.variant === 'destructive' && 'border-red-200 dark:border-red-900',
          )}
        >
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{t.title}</p>
              {t.description != null && (
                <div className="mt-0.5 text-xs text-muted-foreground">{t.description}</div>
              )}
            </div>
            <button
              type="button"
              className="rounded p-0.5 text-muted-foreground hover:text-foreground"
              onClick={() => onDismiss(t.id)}
              aria-label="Dismiss"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}

export { ToasterHost as Toaster }
