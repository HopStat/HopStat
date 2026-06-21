import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  active: boolean
  className?: string
  inBody?: boolean
}

const TYPE_MS = 140
const HOLD_MS = 900
const RESTART_MS = 400

type Phase = 'running' | 'completed'

export function TypewriterStatus({ active, className, inBody = false }: Props) {
  const { t, locale } = useI18n()
  const runningText = t('result.running')
  const completedText = t('result.completed')
  const [phase, setPhase] = useState<Phase>(() => (active ? 'running' : 'completed'))
  const [display, setDisplay] = useState('')

  useEffect(() => {
    setPhase(active ? 'running' : 'completed')
  }, [active])

  useEffect(() => {
    if (phase !== 'running') {
      setDisplay('')
      return
    }

    let cancelled = false
    let index = 0
    let timer: ReturnType<typeof setTimeout>

    const schedule = (fn: () => void, ms: number) => {
      timer = setTimeout(fn, ms)
    }

    const typeStep = () => {
      if (cancelled) return
      if (index < runningText.length) {
        index += 1
        setDisplay(runningText.slice(0, index))
        schedule(typeStep, TYPE_MS)
        return
      }

      schedule(() => {
        if (cancelled) return
        setDisplay('')
        index = 0
        schedule(typeStep, RESTART_MS)
      }, HOLD_MS)
    }

    typeStep()

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [phase, runningText, locale])

  if (phase === 'completed') {
    return (
      <span className={cn('output-terminal__status output-terminal__status--done', className)} aria-live="polite">
        {completedText}
      </span>
    )
  }

  return (
    <span
      className={cn(
        'output-terminal__status inline-flex items-center',
        inBody && 'output-terminal__status--body',
        className,
      )}
      aria-live="polite"
    >
      {display}
      <span
        className={cn(
          'output-terminal__cursor',
          inBody ? 'output-terminal__cursor--body' : 'output-terminal__cursor--header',
        )}
        aria-hidden
      />
    </span>
  )
}
