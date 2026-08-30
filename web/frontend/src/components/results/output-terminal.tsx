import { useEffect, useRef, useState } from 'react'
import { Terminal, Copy, Check } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'
import { isTracerouteHopLine } from '@/lib/result-parse'
import { TypewriterStatus } from './typewriter-status'

interface Props {
  lines: string[]
  isRunning?: boolean
  animateHops?: boolean
  /** Full report to copy instead of the bare output lines. */
  copyText?: () => string
}

export function OutputTerminal({ lines, isRunning, animateHops, copyText }: Props) {
  const { t } = useI18n()
  const bodyRef = useRef<HTMLDivElement>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    const body = bodyRef.current
    if (!body) return
    body.scrollTop = body.scrollHeight
  }, [lines.length, isRunning])

  const handleCopy = async () => {
    await navigator.clipboard.writeText(copyText ? copyText() : lines.join('\n'))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  function renderLine(line: string, i: number) {
    const hopAnim = animateHops && isTracerouteHopLine(line)
    const lineClass = hopAnim ? 'animate-hop-in' : ''
    const parts = line.split(/(\[AS\d+[^\]]*\])/g)
    if (parts.length === 1) {
      return (
        <div
          key={i}
          className={`output-terminal__line whitespace-pre-wrap break-all font-data ${lineClass}`}
          style={hopAnim ? { animationDelay: '0ms' } : undefined}
        >
          {line}
        </div>
      )
    }

    return (
      <div
        key={i}
        className={`output-terminal__line whitespace-pre-wrap break-all font-data ${lineClass}`}
        style={hopAnim ? { animationDelay: '0ms' } : undefined}
      >
        {parts.map((part, j) =>
          part.startsWith('[AS') ? (
            <span key={j} className="output-terminal__asn">{part}</span>
          ) : (
            part
          ),
        )}
      </div>
    )
  }

  return (
    <div className="result-surface output-terminal animate-fade-up">
      <div
        className="output-terminal__header flex items-center gap-2 px-3 py-2 border-b border-border min-h-[2.25rem]"
        aria-busy={isRunning || undefined}
      >
        <Terminal className="output-terminal__prompt w-3.5 h-3.5 text-muted-foreground shrink-0" aria-hidden />
        <TypewriterStatus active={!!isRunning} />
        {lines.length > 0 && (
          <button
            onClick={handleCopy}
            className="ml-auto flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
          >
            {copied ? (
              <><Check className="w-3 h-3 text-brand-accent" /><span>{t('result.copied')}</span></>
            ) : (
              <><Copy className="w-3 h-3" /><span>{t('result.copy')}</span></>
            )}
          </button>
        )}
      </div>

      <div
        ref={bodyRef}
        className="output-terminal__body p-3 sm:p-4 max-h-72 sm:max-h-96 overflow-x-auto overflow-y-auto text-xs sm:text-[13px] leading-5 sm:leading-6"
      >
        {lines.map((line, i) => renderLine(line, i))}
      </div>
    </div>
  )
}
