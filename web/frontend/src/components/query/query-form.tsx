import { useState, useEffect, useMemo } from 'react'
import { Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CardContent } from '@/components/ui/card'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import { useSettings } from '@/contexts/settings-context'
import type { Node } from '@/types/domain'

const commands = [
  { value: 'ping', labelKey: 'cmd.ping' },
  { value: 'traceroute', labelKey: 'cmd.traceroute' },
  { value: 'mtr', labelKey: 'cmd.mtr' },
  { value: 'bgp_route', labelKey: 'cmd.bgp_route' },
  { value: 'as_path', labelKey: 'cmd.as_path' },
]

function hexToHSL(hex: string): { h: number; s: number; l: number } | null {
  const m = hex.match(/^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i)
  if (!m) return null
  let r = parseInt(m[1], 16) / 255, g = parseInt(m[2], 16) / 255, b = parseInt(m[3], 16) / 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  let h = 0, s = 0, l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break
      case g: h = ((b - r) / d + 2) / 6; break
      case b: h = ((r - g) / d + 4) / 6; break
    }
  }
  return { h: Math.round(h * 360), s: Math.round(s * 100), l: Math.round(l * 100) }
}

interface Props {
  onQuerySubmit: (queryId: string) => void
}

export function QueryForm({ onQuerySubmit }: Props) {
  const { t } = useI18n()
  const { settings } = useSettings()
  const [nodes, setNodes] = useState<Node[]>([])
  const [nodeId, setNodeId] = useState('')
  const [command, setCommand] = useState('')
  const [target, setTarget] = useState('')
  const availableCmds = commands.filter(c => {
    if (!nodeId) return true
    const node = nodes.find(n => n.id === parseInt(nodeId))
    if (!node || !node.enabled_cmds?.length) return true
    return node.enabled_cmds.includes(c.value)
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const headerColor = settings.header_color || '#1e293b'
  const hsl = useMemo(() => hexToHSL(headerColor), [headerColor])

  const borderColor = hsl ? `hsl(${hsl.h}, ${Math.min(hsl.s, 30)}%, ${Math.min(hsl.l + 30, 88)}%)` : '#e2e8f0'
  const labelColor = hsl ? `hsl(${hsl.h}, ${Math.min(hsl.s + 10, 60)}%, ${Math.min(hsl.l + 5, 45)}%)` : '#334155'
  const inputBorderFocus = hsl ? `hsl(${hsl.h}, ${Math.min(hsl.s, 50)}%, ${Math.min(hsl.l + 15, 60)}%)` : '#64748b'

  const pingCount = parseInt(settings.ping_count as string) || 5
  const maxHops = parseInt(settings.max_hops as string) || 30
  const mtrCycles = parseInt(settings.mtr_cycles as string) || 10

  useEffect(() => {
    api.get<Node[]>('/nodes').then(setNodes).catch(() => {})
  }, [])

  const canSubmit = nodeId && command && target && !loading

  useEffect(() => {
    if (command && !availableCmds.find(c => c.value === command)) setCommand('')
  }, [nodeId])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    setLoading(true)
    setError('')
    try {
      const options: Record<string, number> = {}
      if (command === 'ping') options.ping_count = pingCount
      if (command === 'traceroute' || command === 'mtr') options.max_hops = maxHops
      if (command === 'mtr') options.mtr_cycles = mtrCycles

      const res = await api.post<{ query_id: string }>('/query', {
        node_id: parseInt(nodeId),
        command,
        target,
        options,
      })
      onQuerySubmit(res.query_id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('query.failed'))
    } finally {
      setLoading(false)
    }
  }

  const focusVars = { '--tw-ring-color': inputBorderFocus } as React.CSSProperties

  return (
    <div className="rounded-xl border-2 bg-card shadow-sm overflow-hidden" style={{ borderColor }}>
      <div className="px-5 py-3 text-white" style={{ backgroundColor: headerColor }}>
        <h2 className="text-sm font-semibold tracking-wide uppercase opacity-90">{t('query.network_diagnostic')}</h2>
      </div>
      <CardContent className="pt-5 pb-5" style={focusVars}>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label style={{ color: labelColor }}>{t('query.node')}</Label>
              <Select value={nodeId} onValueChange={setNodeId}>
                <SelectTrigger><SelectValue placeholder={t('query.select_node')} /></SelectTrigger>
                <SelectContent>
                  {nodes.filter(n => n.active).map(n => (
                    <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label style={{ color: labelColor }}>{t('query.command')}</Label>
              <Select value={command} onValueChange={setCommand}>
                <SelectTrigger><SelectValue placeholder={t('query.select_command')} /></SelectTrigger>
                <SelectContent>
                  {availableCmds.map(c => (
                    <SelectItem key={c.value} value={c.value}>{t(c.labelKey)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label style={{ color: labelColor }}>{t('query.target')}</Label>
              <Input value={target} onChange={e => setTarget(e.target.value)} placeholder={t('query.target_placeholder')} />
            </div>
            <div className="space-y-2">
              <Label className="invisible">{t('query.submit')}</Label>
              <Button type="submit" disabled={!canSubmit} className="w-full text-white shadow-md hover:opacity-90 transition-opacity" style={{ backgroundColor: headerColor }}>
                {loading ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Play className="w-4 h-4 mr-2" />}
                {t('query.submit')}
              </Button>
            </div>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </form>
      </CardContent>
    </div>
  )
}
