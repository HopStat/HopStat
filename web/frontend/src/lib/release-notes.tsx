import type { ReactNode } from 'react'
import { formatReleaseVersion, stripReleaseTitleHeadings } from './release-notes-format'

const INLINE_PATTERN = /(\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g

function splitReleaseSections(markdown: string): string[] {
  const cleaned = stripReleaseTitleHeadings(markdown)
  if (!cleaned) return []
  if (!cleaned.includes('\n\n---\n\n')) return [cleaned]
  return cleaned
    .split('\n\n---\n\n')
    .map(section => section.trim())
    .filter(Boolean)
}

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
          className="text-brand-accent hover:underline"
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
      <ul key={`list-${blockIndex++}`} className="list-disc space-y-0.5 pl-4 text-xs leading-relaxed text-muted-foreground">
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
    if (trimmed.startsWith('# ') && !trimmed.startsWith('## ')) {
      flushList()
      blocks.push(
        <h3 key={`h3-${blockIndex++}`} className="text-sm font-semibold text-foreground">
          {renderInline(trimmed.slice(2), `h1-${blockIndex}`)}
        </h3>,
      )
      continue
    }
    if (trimmed === '---') {
      flushList()
      blocks.push(<hr key={`hr-${blockIndex++}`} className="border-border/70" />)
      continue
    }
    if (trimmed.startsWith('### ')) {
      flushList()
      blocks.push(
        <h4 key={`h4-${blockIndex++}`} className="text-xs font-semibold text-foreground">
          {renderInline(trimmed.slice(4), `h4-${blockIndex}`)}
        </h4>,
      )
      continue
    }
    if (trimmed.startsWith('## ')) {
      flushList()
      blocks.push(
        <h3 key={`h3-${blockIndex++}`} className="text-sm font-semibold text-foreground">
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
      <p key={`p-${blockIndex++}`} className="text-xs leading-relaxed text-muted-foreground">
        {renderInline(trimmed, `p-${blockIndex}`)}
      </p>,
    )
  }
  flushList()

  return (
    <div lang="en" className="space-y-2">
      {blocks}
    </div>
  )
}

export function ReleaseNotesPanel({
  markdown,
  releaseVersions,
}: {
  markdown: string
  releaseVersions?: string[]
}) {
  const sections = splitReleaseSections(markdown)
  const versions = (releaseVersions ?? []).map(formatReleaseVersion).filter(Boolean)
  const aggregated = versions.length > 1 && sections.length > 1

  if (!aggregated) {
    return <ReleaseNotesContent markdown={sections[0] ?? markdown} />
  }

  return (
    <div className="space-y-4">
      {sections.map((section, index) => (
        <section key={versions[index] ?? `section-${index}`} className="release-notes-section">
          {versions[index] ? (
            <h3 className="release-notes-section__version font-data tabular-nums">{versions[index]}</h3>
          ) : null}
          <ReleaseNotesContent markdown={section} />
        </section>
      ))}
    </div>
  )
}
