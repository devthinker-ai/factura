import { useCallback, useState, type KeyboardEvent } from 'react'

/**
 * Arrow/Enter/Space keyboard nav for focusable table rows.
 * Rows should have tabIndex={0} and data-row-index.
 */
export function useTableKeyboardNav(opts: {
  rowCount: number
  onOpen: (index: number) => void
}) {
  const [focusIndex, setFocusIndex] = useState(0)

  const onKeyDown = useCallback(
    (e: KeyboardEvent, index: number) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        const next = Math.min(index + 1, opts.rowCount - 1)
        setFocusIndex(next)
        const el = document.querySelector<HTMLElement>(`[data-row-index="${next}"]`)
        el?.focus()
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        const next = Math.max(index - 1, 0)
        setFocusIndex(next)
        const el = document.querySelector<HTMLElement>(`[data-row-index="${next}"]`)
        el?.focus()
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        opts.onOpen(index)
      }
    },
    [opts],
  )

  return { focusIndex, setFocusIndex, onKeyDown }
}
