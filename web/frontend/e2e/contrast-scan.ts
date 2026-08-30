import type { Page } from '@playwright/test'

export interface Violation {
  path: string
  text: string
  fg: string
  bg: string
  ratio: number
  required: number
  fontPx: number
}

/**
 * Reads the contrast of every visible run of text on the rendered page.
 *
 * This exists because the token-level test in src/lib cannot see a component that hardcodes
 * a colour instead of using a token. Here the browser has already resolved the cascade, so
 * what is measured is what a user actually sees.
 *
 * Deliberately narrow, because a scanner that cries wolf gets switched off:
 *   - text only; borders and other non-text contrast are not asserted
 *   - disabled controls are exempt (WCAG 1.4.3)
 *   - text over a bitmap image is skipped, since no single background colour exists
 *
 * A gradient is resolved into its colour stops and the text is judged against the worst of
 * them: the design uses gradients almost everywhere, so skipping them would blind the scan.
 * Known limitation: only ancestor backgrounds are composited, so a decorative overlay that
 * is a sibling rather than a parent is not accounted for.
 */
export interface ScanResult {
  violations: Violation[]
  /** How many runs of text were actually measured. A scan that examines nothing would
   *  otherwise report a clean page forever. */
  examined: number
}

export async function scanContrast(page: Page): Promise<ScanResult> {
  return page.evaluate(() => {
    type RGBA = [number, number, number, number]

    const parse = (input: string): RGBA | null => {
      const m = input.match(/rgba?\(([^)]+)\)/)
      if (!m) return null
      const parts = m[1].split(/[,\s/]+/).filter(Boolean).map(Number)
      if (parts.length < 3 || parts.some(Number.isNaN)) return null
      return [parts[0], parts[1], parts[2], parts.length > 3 ? parts[3] : 1]
    }

    const over = (fg: RGBA, bg: RGBA): RGBA => {
      const a = fg[3]
      return [
        fg[0] * a + bg[0] * (1 - a),
        fg[1] * a + bg[1] * (1 - a),
        fg[2] * a + bg[2] * (1 - a),
        1,
      ]
    }

    const luminance = (c: RGBA): number => {
      const channel = (v: number) => {
        const s = v / 255
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
      }
      return 0.2126 * channel(c[0]) + 0.7152 * channel(c[1]) + 0.0722 * channel(c[2])
    }

    const ratio = (a: number, b: number) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)

    const hex = (c: RGBA) =>
      '#' + [c[0], c[1], c[2]].map(v => Math.round(v).toString(16).padStart(2, '0')).join('')

    /** Pulls every colour out of a gradient declaration. Chromium serialises stops as
     *  color(srgb r g b / a) with 0..1 components, or as rgb()/rgba(). */
    const gradientStops = (value: string): RGBA[] => {
      const stops: RGBA[] = []
      const srgb = /color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?:\s*\/\s*([\d.]+))?\)/g
      for (let m = srgb.exec(value); m; m = srgb.exec(value)) {
        stops.push([
          Number(m[1]) * 255,
          Number(m[2]) * 255,
          Number(m[3]) * 255,
          m[4] === undefined ? 1 : Number(m[4]),
        ])
      }
      const rgb = /rgba?\(([^)]+)\)/g
      for (let m = rgb.exec(value); m; m = rgb.exec(value)) {
        const c = parse(m[0])
        if (c) stops.push(c)
      }
      return stops
    }

    /** Every background colour that can sit behind the element, nearest layer first. A
     *  gradient contributes one candidate per stop; the caller judges against the worst.
     *  Returns null when a bitmap image is involved and no colour can be derived. */
    const backgroundsOf = (start: Element): RGBA[] | null => {
      const solid: RGBA[] = []
      const translucent: RGBA[] = []
      let node: Element | null = start
      let opaqueFound = false

      while (node && !opaqueFound) {
        const cs = getComputedStyle(node)
        const image = cs.backgroundImage
        if (image && image !== 'none') {
          const stops = gradientStops(image)
          // url(...) with no colour stops: a real image, nothing to measure against.
          if (stops.length === 0 && /url\(/.test(image)) return null
          for (const stop of stops) translucent.push(stop)
        }
        const c = parse(cs.backgroundColor)
        if (c && c[3] > 0) {
          if (c[3] === 1) {
            solid.push(c)
            opaqueFound = true
          } else {
            translucent.push(c)
          }
        }
        node = node.parentElement
      }

      // The canvas under everything. Chromium paints white when nothing else does.
      const base: RGBA = solid.length ? solid[solid.length - 1] : [255, 255, 255, 1]
      const candidates: RGBA[] = [base]
      for (const layer of translucent) candidates.push(over(layer, base))
      return candidates
    }

    const effectiveOpacity = (start: Element): number => {
      let acc = 1
      let node: Element | null = start
      while (node) {
        const o = parseFloat(getComputedStyle(node).opacity)
        if (!Number.isNaN(o)) acc *= o
        node = node.parentElement
      }
      return acc
    }

    const describe = (el: Element): string => {
      const parts: string[] = []
      let node: Element | null = el
      for (let depth = 0; node && depth < 4; depth++) {
        let part = node.tagName.toLowerCase()
        if (node.id) part += `#${node.id}`
        else if (typeof node.className === 'string' && node.className.trim()) {
          part += `.${node.className.trim().split(/\s+/).slice(0, 2).join('.')}`
        }
        parts.unshift(part)
        node = node.parentElement
      }
      return parts.join(' > ')
    }

    const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TITLE', 'HEAD', 'HTML'])
    const out: Violation[] = []
    let examined = 0

    /** Each run of text an element paints, with the colour it is painted in. Fields carry
     *  their value and placeholder as rendered text with no text node behind them, so a
     *  scan that only walks text nodes misses most of a form. */
    const runsOf = (el: Element): Array<{ text: string; color: string }> => {
      const tag = el.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA') {
        const field = el as HTMLInputElement | HTMLTextAreaElement
        // Field types that paint no text: their value is a position or a swatch, and the
        // element's colour says nothing about legibility.
        const type = (field as HTMLInputElement).type
        const TEXTLESS = ['hidden', 'checkbox', 'radio', 'range', 'color', 'file', 'image']
        if (TEXTLESS.includes(type)) return []
        const runs: Array<{ text: string; color: string }> = []
        const own = getComputedStyle(field).color
        if (field.value.trim()) runs.push({ text: field.value.trim(), color: own })
        if (field.placeholder?.trim()) {
          runs.push({
            text: field.placeholder.trim(),
            color: getComputedStyle(field, '::placeholder').color || own,
          })
        }
        return runs
      }

      // Only text this element owns, so a wrapper is not blamed for its child's colour.
      const text = Array.from(el.childNodes)
        .filter(n => n.nodeType === Node.TEXT_NODE)
        .map(n => n.textContent ?? '')
        .join(' ')
        .trim()
      return text ? [{ text, color: getComputedStyle(el).color }] : []
    }

    for (const el of Array.from(document.querySelectorAll('*'))) {
      if (SKIP_TAGS.has(el.tagName)) continue

      const runs = runsOf(el)
      if (runs.length === 0) continue

      // aria-hidden is deliberately NOT a reason to skip: it hides text from the
      // accessibility tree, not from the eye, and contrast is about what is seen.
      // WCAG 1.4.3 does exempt inactive controls.
      if (el.closest('[disabled], [aria-disabled="true"], [data-disabled]')) continue

      const rect = el.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) continue

      const cs = getComputedStyle(el)
      if (cs.visibility !== 'visible') continue
      // Gradient-filled text has no flat colour to measure.
      if (cs.webkitTextFillColor && parse(cs.webkitTextFillColor)?.[3] === 0) continue
      if (cs.backgroundClip === 'text' || cs.webkitBackgroundClip === 'text') continue

      const opacity = effectiveOpacity(el)
      if (opacity === 0) continue

      const backgrounds = backgroundsOf(el)
      if (!backgrounds) continue

      const fontPx = parseFloat(cs.fontSize)
      const weight = parseInt(cs.fontWeight, 10) || 400
      const large = fontPx >= 24 || (fontPx >= 18.66 && weight >= 700)
      const required = large ? 3 : 4.5

      for (const run of runs) {
        const rawFg = parse(run.color)
        if (!rawFg) continue
        examined++

        // Judged against the least forgiving background the element can sit on.
        let worst = { ratio: Infinity, bg: backgrounds[0], fg: backgrounds[0] }
        for (const bg of backgrounds) {
          const fg = over([rawFg[0], rawFg[1], rawFg[2], rawFg[3] * opacity], bg)
          const got = ratio(luminance(fg), luminance(bg))
          if (got < worst.ratio) worst = { ratio: got, bg, fg }
        }

        if (worst.ratio + 0.005 < required) {
          out.push({
            path: describe(el),
            text: run.text.slice(0, 60),
            fg: hex(worst.fg),
            bg: hex(worst.bg),
            ratio: Math.round(worst.ratio * 100) / 100,
            required,
            fontPx,
          })
        }
      }
    }

    return { violations: out, examined }
  })
}

export function formatViolations(violations: Violation[]): string {
  return violations
    .map(
      v =>
        `  ${v.ratio}:1 (needs ${v.required}:1) ${v.fg} on ${v.bg} @${v.fontPx}px\n` +
        `    "${v.text}"\n    ${v.path}`,
    )
    .join('\n')
}
