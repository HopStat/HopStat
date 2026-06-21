import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface PageHeaderProps {
  title: string
  description?: string
  eyebrow?: string
  children?: ReactNode
  className?: string
}

export function PageHeader({
  title,
  description,
  eyebrow = 'Admin',
  children,
  className,
}: PageHeaderProps) {
  return (
    <div className={cn('admin-page-header', className)}>
      <div>
        {eyebrow && <p className="admin-page-header__eyebrow">{eyebrow}</p>}
        <h1 className="admin-page-header__title">{title}</h1>
        {description && <p className="admin-page-header__description">{description}</p>}
      </div>
      {children && <div className="admin-page-header__actions">{children}</div>}
    </div>
  )
}
