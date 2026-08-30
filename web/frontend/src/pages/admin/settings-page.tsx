import { useEffect, useState, useRef, type CSSProperties } from 'react'
import { Globe2, Save, Upload, User } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { QueryErrorAlert } from '@/components/results/query-error-alert'
import { api } from '@/lib/api-client'
import { useI18n, getLocaleLabelKey } from '@/contexts/i18n-context'
import { useSettings } from '@/contexts/settings-context'
import { HeaderColorPreview } from '@/components/admin/header-color-preview'
import { SUPPORTED_LOCALES, parseActiveLanguages, type Locale } from '@/i18n/index'
import type { GeoIPStatus } from '@/types/domain'

interface Settings {
  site_name: string
  site_description: string
  logo_path: string
  header_color: string
  url_website: string
  url_peeringdb: string
  url_contact: string
  url_terms: string
  url_privacy: string
  ping_count: string
  max_hops: string
  traceroute_max_timeouts: string
  active_languages: string
}

interface Account {
  id: number
  email: string
}

function clampSetting(value: string, min: number, max: number, fallback: number): number {
  const n = parseInt(value, 10)
  if (Number.isNaN(n)) return fallback
  return Math.min(max, Math.max(min, n))
}

function rangeSliderStyle(value: number, min: number, max: number, fillColor: string): CSSProperties {
  const percent = max === min ? 0 : ((value - min) / (max - min)) * 100
  return {
    '--range-fill': fillColor,
    '--range-percent': `${percent}%`,
  } as CSSProperties
}

function QueryDefaultSlider({
  label,
  hint,
  min,
  max,
  value,
  onChange,
  fillColor,
}: {
  label: string
  hint: string
  min: number
  max: number
  value: number
  onChange: (value: number) => void
  fillColor: string
}) {
  return (
    <div className="min-w-0 space-y-3 rounded-lg border border-border bg-muted/20 px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <Label className="leading-snug">{label}</Label>
        <span className="inline-flex min-w-[2rem] items-center justify-center rounded-md border border-border bg-background px-2 py-0.5 text-sm font-data font-semibold tabular-nums">
          {value}
        </span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={1}
        value={value}
        onChange={e => onChange(parseInt(e.target.value, 10))}
        style={rangeSliderStyle(value, min, max, fillColor)}
        className="settings-range w-full cursor-pointer"
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuenow={value}
      />
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  )
}

export function SettingsPage() {
  const { t } = useI18n()
  const { reload: reloadSiteSettings } = useSettings()
  const [settings, setSettings] = useState<Settings>({
    site_name: '', site_description: '', logo_path: '', header_color: '#1e293b',
    url_website: '', url_peeringdb: '', url_contact: '', url_terms: '', url_privacy: '',
    ping_count: '5', max_hops: '30', traceroute_max_timeouts: '5', active_languages: 'en,tr',
  })
  const [account, setAccount] = useState({ email: '', currentPassword: '', newPassword: '', confirmPassword: '' })
  const [saved, setSaved] = useState(false)
  const [accountSaved, setAccountSaved] = useState(false)
  const [accountError, setAccountError] = useState('')
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  // MaxMind credentials live in the database, not the config file, so they can be changed
  // here. The stored licence key is never sent back — only whether one exists.
  const [geoip, setGeoip] = useState<GeoIPStatus | null>(null)
  const [geoipForm, setGeoipForm] = useState({ accountId: '', licenseKey: '', interval: '' })
  const [geoipSaved, setGeoipSaved] = useState(false)
  const [geoipError, setGeoipError] = useState('')

  useEffect(() => {
    api.get<Settings>('/admin/settings').then(s => {
      if (s) setSettings({ ...s, active_languages: s.active_languages || 'en,tr' })
    }).catch(() => {})
    api.get<Account>('/admin/account').then(a => {
      if (a?.email) setAccount(prev => ({ ...prev, email: a.email }))
    }).catch(() => {})
    api.get<GeoIPStatus>('/admin/geoip/status').then(applyGeoipStatus).catch(() => {})
  }, [])

  function applyGeoipStatus(status: GeoIPStatus | null) {
    if (!status) return
    setGeoip(status)
    setGeoipForm({ accountId: status.account_id, licenseKey: '', interval: status.update_interval })
  }

  const handleGeoipSave = async (clear = false) => {
    setGeoipError('')
    try {
      const status = await api.put<GeoIPStatus>('/admin/geoip/config', clear
        ? { clear_credentials: true }
        : {
            account_id: geoipForm.accountId,
            license_key: geoipForm.licenseKey,
            update_interval: geoipForm.interval,
          })
      applyGeoipStatus(status)
      setGeoipSaved(true)
      setTimeout(() => setGeoipSaved(false), 2000)
    } catch (err) {
      setGeoipError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleSave = async () => {
    try {
      await api.put('/admin/settings', settings)
      reloadSiteSettings()
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch { /* ignore */ }
  }

  const handleAccountSave = async () => {
    setAccountError('')
    if (account.newPassword && account.newPassword !== account.confirmPassword) {
      setAccountError(t('admin.password_mismatch'))
      return
    }
    try {
      const updated = await api.put<Account>('/admin/account', {
        email: account.email,
        current_password: account.currentPassword,
        new_password: account.newPassword || undefined,
      })
      setAccount(prev => ({
        ...prev,
        email: updated.email,
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
      }))
      setAccountSaved(true)
      setTimeout(() => setAccountSaved(false), 2000)
    } catch (err) {
      setAccountError(err instanceof Error ? err.message : t('admin.error'))
    }
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      const formData = new FormData()
      formData.append('logo', file)
      const res = await fetch('/api/v1/admin/settings/logo', {
        method: 'POST',
        credentials: 'include',
        body: formData,
      })
      if (!res.ok) throw new Error(`Upload failed: HTTP ${res.status}`)
      const json = await res.json()
      if (json.data?.logo_path) {
        setSettings(s => ({ ...s, logo_path: json.data.logo_path }))
        reloadSiteSettings()
      }
    } catch { /* ignore */ }
    setUploading(false)
    if (fileRef.current) fileRef.current.value = ''
  }

  const toggleLanguage = (code: Locale) => {
    const active = new Set(parseActiveLanguages(settings.active_languages))
    if (active.has(code)) {
      if (active.size <= 1) return
      active.delete(code)
    } else {
      active.add(code)
    }
    const ordered = SUPPORTED_LOCALES.filter(l => active.has(l))
    setSettings(s => ({ ...s, active_languages: ordered.join(',') }))
  }

  const urlFields: { key: keyof Settings; labelKey: string; placeholder: string }[] = [
    { key: 'url_website', labelKey: 'admin.url_website', placeholder: 'https://example.com' },
    { key: 'url_peeringdb', labelKey: 'admin.url_peeringdb', placeholder: 'https://peeringdb.com/asn/XXXX' },
    { key: 'url_contact', labelKey: 'admin.url_contact', placeholder: 'mailto:noc@example.com' },
    { key: 'url_terms', labelKey: 'admin.url_terms', placeholder: 'https://example.com/terms' },
    { key: 'url_privacy', labelKey: 'admin.url_privacy', placeholder: 'https://example.com/privacy' },
  ]

  const tracerouteMaxTimeouts = clampSetting(settings.traceroute_max_timeouts, 1, 20, 5)
  const pingCount = clampSetting(settings.ping_count, 1, 50, 5)
  const maxHops = clampSetting(settings.max_hops, 1, 64, 30)
  const headerColor = settings.header_color || '#1e293b'

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.settings')} eyebrow={t('admin.title')} />

      <Card>
        <CardHeader><CardTitle>{t('admin.site_identity')}</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t('admin.site_name')}</Label>
              <Input value={settings.site_name} onChange={e => setSettings(s => ({ ...s, site_name: e.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>{t('admin.site_description')}</Label>
              <Input value={settings.site_description} onChange={e => setSettings(s => ({ ...s, site_description: e.target.value }))} />
            </div>
          </div>
          <div className="space-y-2">
            <Label>{t('admin.logo')}</Label>
            <div className="flex items-center gap-4">
              {settings.logo_path && (
                <img
                  key={settings.logo_path}
                  src={settings.logo_path}
                  alt={t('admin.logo')}
                  title={t('admin.logo_preview_hint')}
                  className="h-12 w-12 object-contain shrink-0"
                />
              )}
              <label className="relative inline-flex items-center justify-center gap-2 whitespace-nowrap text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring border border-input bg-background hover:bg-accent hover:text-accent-foreground h-9 rounded-md px-3 cursor-pointer disabled:pointer-events-none disabled:opacity-50 overflow-hidden">
                <Upload className="w-4 h-4" />
                {uploading ? t('admin.uploading') : settings.logo_path ? t('admin.logo_change') : t('admin.logo_choose')}
                <input ref={fileRef} type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp"
                  className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
                  onChange={handleFileChange} disabled={uploading} />
              </label>
              {settings.logo_path && (
                <Button variant="ghost" size="sm" onClick={() => setSettings(s => ({ ...s, logo_path: '' }))}>{t('admin.logo_remove')}</Button>
              )}
            </div>
            <p className="text-xs text-muted-foreground">{t('admin.logo_hint')}</p>
          </div>
          <div className="space-y-2">
            <Label>{t('admin.header_color')}</Label>
            <div className="flex items-center gap-3 p-3 rounded-xl border border-border/80 bg-muted/30">
              <input
                type="color"
                value={settings.header_color || '#1e293b'}
                onChange={e => setSettings(s => ({ ...s, header_color: e.target.value }))}
                className="h-11 w-14 rounded-xl border border-input cursor-pointer bg-transparent"
              />
              <Input
                value={settings.header_color || '#1e293b'}
                onChange={e => setSettings(s => ({ ...s, header_color: e.target.value }))}
                className="w-36 font-data"
                placeholder="#1e293b"
              />
              <HeaderColorPreview color={headerColor} />
            </div>
            <p className="text-xs text-muted-foreground">{t('admin.header_color_hint')}</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.active_languages')}</CardTitle></CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {SUPPORTED_LOCALES.map(code => {
              const active = parseActiveLanguages(settings.active_languages).includes(code)
              return (
                <button
                  key={code}
                  type="button"
                  onClick={() => toggleLanguage(code)}
                  className={`inline-flex items-center gap-2 rounded-xl border px-3 py-2 text-sm transition-colors ${
                    active
                      ? 'border-brand-accent-ui/40 bg-brand/10 text-brand-accent'
                      : 'border-border text-muted-foreground hover:border-border/80'
                  }`}
                >
                  <span className="font-semibold text-xs uppercase">{code}</span>
                  <span>{t(getLocaleLabelKey(code))}</span>
                </button>
              )
            })}
          </div>
          <p className="text-xs text-muted-foreground">{t('admin.active_languages_hint')}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.query_defaults')}</CardTitle></CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <QueryDefaultSlider
              label={t('admin.ping_count')}
              hint={t('admin.ping_count_hint')}
              min={1}
              max={50}
              value={pingCount}
              onChange={v => setSettings(s => ({ ...s, ping_count: String(v) }))}
              fillColor={headerColor}
            />
            <QueryDefaultSlider
              label={t('admin.max_hops')}
              hint={t('admin.max_hops_hint')}
              min={1}
              max={64}
              value={maxHops}
              onChange={v => setSettings(s => ({ ...s, max_hops: String(v) }))}
              fillColor={headerColor}
            />
            <QueryDefaultSlider
              label={t('admin.traceroute_max_timeouts')}
              hint={t('admin.traceroute_max_timeouts_hint')}
              min={1}
              max={20}
              value={tracerouteMaxTimeouts}
              onChange={v => setSettings(s => ({ ...s, traceroute_max_timeouts: String(v) }))}
              fillColor={headerColor}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.footer_links')}</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          {urlFields.map(f => (
            <div key={f.key} className="space-y-1">
              <Label>{t(f.labelKey)}</Label>
              <Input
                placeholder={f.placeholder}
                value={settings[f.key]}
                onChange={e => setSettings(s => ({ ...s, [f.key]: e.target.value }))}
              />
            </div>
          ))}
          <p className="text-xs text-muted-foreground">{t('admin.footer_links_hint')}</p>
        </CardContent>
      </Card>

      <Button onClick={handleSave} disabled={saved}>
        <Save className="w-4 h-4 mr-1" />
        {saved ? t('admin.saved') : t('admin.save_settings')}
      </Button>

      <Card>
        <CardHeader><CardTitle>{t('admin.geoip_maxmind')}</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">{t('admin.geoip_maxmind_hint')}</p>

          {geoip && (
            <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
              <span>
                {geoip.configured
                  ? t('admin.geoip_status_configured')
                  : t('admin.geoip_status_not_configured')}
              </span>
              <span>
                {t('admin.geoip_databases_loaded')}:{' '}
                {geoip.asn_loaded && geoip.city_loaded ? 'ASN + City' : geoip.asn_loaded ? 'ASN' : geoip.city_loaded ? 'City' : '—'}
              </span>
              <span>
                {t('admin.geoip_last_download')}:{' '}
                {geoip.last_download ? new Date(geoip.last_download).toLocaleString() : t('admin.geoip_never')}
              </span>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t('admin.geoip_account_id')}</Label>
              <Input
                inputMode="numeric"
                placeholder="123456"
                value={geoipForm.accountId}
                onChange={e => setGeoipForm(g => ({ ...g, accountId: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label>{t('admin.geoip_license_key')}</Label>
              <Input
                type="password"
                autoComplete="off"
                value={geoipForm.licenseKey}
                onChange={e => setGeoipForm(g => ({ ...g, licenseKey: e.target.value }))}
              />
              <p className="text-xs text-muted-foreground">
                {geoip?.license_key_set
                  ? t('admin.geoip_license_key_stored')
                  : t('admin.geoip_license_key_none')}
              </p>
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('admin.geoip_update_interval')}</Label>
            <Input
              placeholder="72h"
              value={geoipForm.interval}
              onChange={e => setGeoipForm(g => ({ ...g, interval: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">{t('admin.geoip_update_interval_hint')}</p>
          </div>

          {geoipError && <QueryErrorAlert message={geoipError} />}

          <div className="flex flex-wrap gap-2">
            <Button onClick={() => handleGeoipSave()} disabled={geoipSaved}>
              <Globe2 className="w-4 h-4 mr-1" />
              {geoipSaved ? t('admin.saved') : t('admin.geoip_save')}
            </Button>
            {geoip?.license_key_set && (
              <Button variant="outline" onClick={() => handleGeoipSave(true)}>
                {t('admin.geoip_clear')}
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.account')}</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">{t('admin.account_hint')}</p>
          <div className="space-y-2">
            <Label>{t('admin.email')}</Label>
            <Input
              type="email"
              value={account.email}
              onChange={e => setAccount(a => ({ ...a, email: e.target.value }))}
            />
          </div>
          <div className="space-y-2">
            <Label>{t('admin.current_password')}</Label>
            <Input
              type="password"
              autoComplete="current-password"
              value={account.currentPassword}
              onChange={e => setAccount(a => ({ ...a, currentPassword: e.target.value }))}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t('admin.new_password')}</Label>
              <Input
                type="password"
                autoComplete="new-password"
                placeholder={t('admin.new_password_placeholder')}
                value={account.newPassword}
                onChange={e => setAccount(a => ({ ...a, newPassword: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label>{t('admin.confirm_password')}</Label>
              <Input
                type="password"
                autoComplete="new-password"
                value={account.confirmPassword}
                onChange={e => setAccount(a => ({ ...a, confirmPassword: e.target.value }))}
              />
            </div>
          </div>
          {accountError && <QueryErrorAlert message={accountError} />}
          <Button onClick={handleAccountSave} disabled={accountSaved || !account.currentPassword}>
            <User className="w-4 h-4 mr-1" />
            {accountSaved ? t('admin.account_saved') : t('admin.save_account')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
