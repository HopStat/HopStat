export function absoluteAssetUrl(path: string): string {
  if (!path) return ''
  if (/^https?:\/\//i.test(path)) return path
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${window.location.origin}${normalized}`
}

export function upsertMeta(attr: 'name' | 'property', key: string, content?: string) {
  if (!content?.trim()) return
  const selector = `meta[${attr}="${key}"]`
  let el = document.querySelector<HTMLMetaElement>(selector)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute(attr, key)
    document.head.appendChild(el)
  }
  el.content = content.trim()
}

export function upsertLink(rel: string, href?: string, attrs?: Record<string, string>) {
  if (!href?.trim()) return
  const selectorParts = [`link[rel="${rel}"]`]
  if (attrs?.sizes) selectorParts.push(`[sizes="${attrs.sizes}"]`)
  let el = document.querySelector<HTMLLinkElement>(selectorParts.join(''))
  if (!el) {
    el = document.createElement('link')
    el.rel = rel
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v)
    }
    document.head.appendChild(el)
  }
  el.href = href.trim()
}

export function upsertJsonLd(id: string, data: Record<string, unknown> | null) {
  const existing = document.getElementById(id)
  if (!data) {
    existing?.remove()
    return
  }
  let el = existing as HTMLScriptElement | null
  if (!el) {
    el = document.createElement('script')
    el.id = id
    el.type = 'application/ld+json'
    document.head.appendChild(el)
  }
  el.textContent = JSON.stringify(data)
}
