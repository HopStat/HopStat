import { useEffect, useState, useRef } from 'react'
import { Save, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'

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
  mtr_cycles: string
}

export function SettingsPage() {
  const { t } = useI18n()
  const [settings, setSettings] = useState<Settings>({
    site_name: '', site_description: '', logo_path: '', header_color: '#1e293b',
    url_website: '', url_peeringdb: '', url_contact: '', url_terms: '', url_privacy: '',
    ping_count: '5', max_hops: '30', mtr_cycles: '10',
  })
  const [saved, setSaved] = useState(false)
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    api.get<Settings>('/admin/settings').then(s => {
      if (s) setSettings(s as Settings)
    }).catch(() => {})
  }, [])

  const handleSave = async () => {
    try {
      await api.put('/admin/settings', settings)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch { /* ignore */ }
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      const formData = new FormData()
      formData.append('logo', file)
      const token = localStorage.getItem('jwt_token')
      const res = await fetch('/api/v1/admin/settings/logo', {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      })
      const json = await res.json()
      if (json.data?.logo_path) {
        setSettings(s => ({ ...s, logo_path: json.data.logo_path }))
      }
    } catch { /* ignore */ }
    setUploading(false)
  }

  const urlFields: { key: keyof Settings; labelKey: string; placeholder: string }[] = [
    { key: 'url_website', labelKey: 'admin.url_website', placeholder: 'https://example.com' },
    { key: 'url_peeringdb', labelKey: 'admin.url_peeringdb', placeholder: 'https://peeringdb.com/asn/XXXX' },
    { key: 'url_contact', labelKey: 'admin.url_contact', placeholder: 'mailto:noc@example.com' },
    { key: 'url_terms', labelKey: 'admin.url_terms', placeholder: 'https://example.com/terms' },
    { key: 'url_privacy', labelKey: 'admin.url_privacy', placeholder: 'https://example.com/privacy' },
  ]

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t('admin.settings')}</h1>

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
                <img src={settings.logo_path} alt={t('admin.logo')} className="h-12 w-12 object-contain rounded border" />
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
            <div className="flex items-center gap-2">
              <input type="color" value={settings.header_color || '#1e293b'} onChange={e => setSettings(s => ({ ...s, header_color: e.target.value }))}
                className="h-9 w-12 rounded border cursor-pointer" />
              <Input value={settings.header_color || '#1e293b'} onChange={e => setSettings(s => ({ ...s, header_color: e.target.value }))}
                className="w-28 font-mono" placeholder="#1e293b" />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.query_defaults')}</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label>{t('admin.ping_count')}</Label>
              <Input type="number" min={1} max={50} value={settings.ping_count} onChange={e => setSettings(s => ({ ...s, ping_count: e.target.value }))} />
              <p className="text-xs text-muted-foreground">{t('admin.ping_count_hint')}</p>
            </div>
            <div className="space-y-2">
              <Label>{t('admin.max_hops')}</Label>
              <Input type="number" min={1} max={64} value={settings.max_hops} onChange={e => setSettings(s => ({ ...s, max_hops: e.target.value }))} />
              <p className="text-xs text-muted-foreground">{t('admin.max_hops_hint')}</p>
            </div>
            <div className="space-y-2">
              <Label>{t('admin.mtr_cycles')}</Label>
              <Input type="number" min={1} max={100} value={settings.mtr_cycles} onChange={e => setSettings(s => ({ ...s, mtr_cycles: e.target.value }))} />
              <p className="text-xs text-muted-foreground">{t('admin.mtr_cycles_hint')}</p>
            </div>
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
    </div>
  )
}
