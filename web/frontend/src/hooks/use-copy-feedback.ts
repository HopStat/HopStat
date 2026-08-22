import { useCallback, useState } from 'react'

/**
 * Copy state shared by every way of copying one result, so clicking the result itself
 * gives the same acknowledgement as pressing the button.
 */
export function useCopyFeedback(text: () => string) {
  const [copied, setCopied] = useState(false)

  const copy = useCallback(() => {
    navigator.clipboard.writeText(text())
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch(() => {})
  }, [text])

  return { copied, copy }
}

/** True when the click was the end of a text selection rather than a plain click. */
export function isTextSelectionClick(): boolean {
  return (window.getSelection()?.toString().trim().length ?? 0) > 0
}
