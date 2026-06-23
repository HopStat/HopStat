import type { ReactNode } from 'react'

const INLINE_PATTERN = /(\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g

function renderInline(text: string, keyPrefix: string): ReactNode[] {
  const parts = text.split(INLINE_PATTERN).filter(part => part !== '')
  return parts.map((part, index) => {
    const key = `${keyPrefix}-${index}`
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={key}>{part.slice(2, -2)}</strong>
    }
    const linkMatch = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(part)
    if (linkMatch) {
      return (
        <a
          key={key}
          href={linkMatch[2]}
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand hover:underline"
        >
          {linkMatch[1]}
        </a>
      )
    }
    return part
  })
}

export function ReleaseNotesContent({ markdown }: { markdown: string }) {
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  const blocks: ReactNode[] = []
  let listItems: string[] = []
  let blockIndex = 0

  const flushList = () => {
    if (listItems.length === 0) return
    blocks.push(
      <ul key={`list-${blockIndex++}`} className="list-disc space-y-1 pl-5">
        {listItems.map((item, index) => (
          <li key={`item-${index}`}>{renderInline(item, `item-${index}`)}</li>
        ))}
      </ul>,
    )
    listItems = []
  }

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) {
      flushList()
      continue
    }
    if (trimmed.startsWith('### ')) {
      flushList()
      blocks.push(
        <h4 key={`h4-${blockIndex++}`} className="text-sm font-semibold text-foreground">
          {renderInline(trimmed.slice(4), `h4-${blockIndex}`)}
        </h4>,
      )
      continue
    }
    if (trimmed.startsWith('## ')) {
      flushList()
      blocks.push(
        <h3 key={`h3-${blockIndex++}`} className="text-base font-semibold text-foreground">
          {renderInline(trimmed.slice(3), `h3-${blockIndex}`)}
        </h3>,
      )
      continue
    }
    if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
      listItems.push(trimmed.slice(2))
      continue
    }
    flushList()
    blocks.push(
      <p key={`p-${blockIndex++}`} className="text-sm leading-relaxed text-muted-foreground">
        {renderInline(trimmed, `p-${blockIndex}`)}
      </p>,
    )
  }
  flushList()

  return (
    <div lang="en" className="space-y-3">
      {blocks}
    </div>
  )
}
