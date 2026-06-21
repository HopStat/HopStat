import { AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Props {
  message: string
  className?: string
}

export function QueryErrorAlert({ message, className }: Props) {
  if (!message.trim()) return null

  return (
    <div
      role="alert"
      className={cn(
        'flex items-start gap-3 rounded-lg border border-destructive/25 bg-destructive/[0.06] px-4 py-3.5 shadow-sm',
        'ring-1 ring-inset ring-destructive/10 animate-fade-up',
        className,
      )}
    >
      <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-destructive/10">
        <AlertCircle className="h-3.5 w-3.5 text-destructive" aria-hidden />
      </span>
      <p className="text-sm leading-relaxed text-destructive pt-0.5">{message}</p>
    </div>
  )
}
