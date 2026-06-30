import { describe, expect, it } from 'vitest'
import { communitySeverityLabel } from './community-severity'

// Mimics the i18n lookup: returns the human label, or the key when missing.
const labels: Record<string, string> = {
  'admin.severity_info': 'Info',
  'admin.severity_success': 'Success',
  'admin.severity_warning': 'Warning',
  'admin.severity_alert': 'Alert',
}
const t = (key: string) => labels[key] ?? key

describe('communitySeverityLabel', () => {
  it('uppercases labels with the English locale, not the active UI language', () => {
    // Regression: under Turkish CSS casing "Warning" became "WARNİNG"
    // because lowercase `i` uppercases to the dotted `İ`.
    expect(communitySeverityLabel('warning', t)).toBe('WARNING')
  })

  it('uppercases every known severity', () => {
    expect(communitySeverityLabel('info', t)).toBe('INFO')
    expect(communitySeverityLabel('success', t)).toBe('SUCCESS')
    expect(communitySeverityLabel('alert', t)).toBe('ALERT')
  })

  it('maps the reject alias onto alert', () => {
    expect(communitySeverityLabel('reject', t)).toBe('ALERT')
  })

  it('falls back to the raw severity when no translation exists', () => {
    expect(communitySeverityLabel('critical', t)).toBe('CRITICAL')
  })
})
