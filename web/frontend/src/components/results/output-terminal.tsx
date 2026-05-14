import { useEffect, useRef, useState } from 'react'
import { Terminal, Copy, Check } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  lines: string[]
  isRunning?: boolean
  accentColor?: string
}

export function OutputTerminal({ lines, isRunning, accentColor = '#1e293b' }: Props) {
  const { t } = useI18n()
  const bottomRef = useRef<HTMLDivElement>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines.length])

  const handleCopy = async () => {
    await navigator.clipboard.writeText(lines.join('\n'))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  function renderLine(line: string, i: number) {
    // Highlight [AS...] tags in cyan
    const asMatch = line.match(/^(.*)(\[AS\d+.*?\])(.*)$/)
    if (!asMatch) return <div key={i} className="text-gray-200 whitespace-pre-wrap break-all">{line}</div>

    return (
      <div key={i} className="text-gray-200 whitespace-pre-wrap break-all">
        {asMatch[1]}<span className="text-cyan-400">{asMatch[2]}</span>{asMatch[3]}
      </div>
    )
  }

  return (
    <div className="rounded-lg overflow-hidden border-2 shadow-lg" style={{ borderColor: accentColor }}>
      <div className="flex items-center gap-2 px-3 py-2 border-b" style={{ backgroundColor: accentColor, borderColor: accentColor }}>
        <Terminal className="w-3.5 h-3.5 text-white/70" />
        <span className="text-xs text-white/70 font-medium tracking-wide">{t('result.output')}</span>
        {isRunning && (
          <span className="flex items-center gap-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
            <span className="text-xs text-green-400">{t('result.running')}</span>
          </span>
        )}
        {!isRunning && lines.length > 0 && (
          <span className="text-xs text-white/40">{lines.length} {t('result.lines')}</span>
        )}
        {lines.length > 0 && (
          <button
            onClick={handleCopy}
            className="ml-auto flex items-center gap-1 px-2 py-0.5 rounded text-xs text-white/50 hover:text-white hover:bg-white/10 transition-colors"
          >
            {copied ? (
              <><Check className="w-3.5 h-3.5 text-green-400" /><span className="text-green-400">{t('result.copied')}</span></>
            ) : (
              <><Copy className="w-3.5 h-3.5" /><span>{t('result.copy')}</span></>
            )}
          </button>
        )}
      </div>

      <div className="bg-gray-950 p-4 max-h-80 overflow-y-auto font-mono text-[13px] leading-6">
        {lines.length === 0 && isRunning && (
          <span className="text-gray-500">{t('result.waiting')}</span>
        )}
        {lines.map((line, i) => renderLine(line, i))}
        {isRunning && (
          <span className="inline-block w-2 h-4 bg-green-400 animate-pulse ml-0.5 align-text-bottom" />
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
