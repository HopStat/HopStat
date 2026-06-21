import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface AdminPanelProps {
  children: ReactNode
  className?: string
  padded?: boolean
}

export function AdminPanel({ children, className, padded = true }: AdminPanelProps) {
  return (
    <div className={cn('admin-panel', padded && 'admin-panel--padded', className)}>
      {children}
    </div>
  )
}
