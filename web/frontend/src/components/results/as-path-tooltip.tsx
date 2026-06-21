import { useCallback, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'

interface Props {
  lines: string[]
  placement?: 'top' | 'bottom'
  className?: string
  children: ReactNode
}

export function AsPathTooltip({ lines, placement = 'top', className, children }: Props) {
  const anchorRef = useRef<HTMLSpanElement>(null)
  const [visible, setVisible] = useState(false)
  const [coords, setCoords] = useState({ x: 0, y: 0 })

  const show = useCallback(() => {
    const el = anchorRef.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    setCoords({
      x: rect.left + rect.width / 2,
      y: placement === 'top' ? rect.top - 8 : rect.bottom + 8,
    })
    setVisible(true)
  }, [placement])

  const hide = useCallback(() => setVisible(false), [])

  if (lines.length === 0) {
    return <span className={className}>{children}</span>
  }

  return (
    <>
      <span
        ref={anchorRef}
        className={cn('inline-flex', className)}
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
      >
        {children}
      </span>
      {visible && createPortal(
        <div
          role="tooltip"
          className={cn(
            'as-path-tooltip pointer-events-none fixed z-[200] w-max max-w-[24rem] truncate whitespace-nowrap px-2.5 py-1 text-[11px] font-normal normal-case leading-none rounded-md -translate-x-1/2',
            placement === 'top' && '-translate-y-full',
          )}
          style={{ left: coords.x, top: coords.y }}
        >
          {lines[0]}
        </div>,
        document.body,
      )}
    </>
  )
}
