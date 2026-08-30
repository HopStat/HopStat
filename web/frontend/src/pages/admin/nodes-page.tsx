import { useState, useEffect } from 'react'
import { Plus, Pencil, Trash2, Zap, Star } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { PageHeader } from '@/components/ui/page-header'
import { AdminPanel } from '@/components/admin/admin-panel'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { Node } from '@/types/domain'

const SUPPORTED_NODE_CMDS = new Set(['ping', 'traceroute', 'bgp_route'])

function normalizeEnabledCmds(cmds: string[]): string[] {
  const seen = new Set<string>()
  return cmds.filter(cmd => {
    if (!SUPPORTED_NODE_CMDS.has(cmd) || seen.has(cmd)) return false
    seen.add(cmd)
    return true
  })
}

export function NodesPage() {
  const { t } = useI18n()
  const [nodes, setNodes] = useState<Node[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editNode, setEditNode] = useState<Node | null>(null)
  const [form, setForm] = useState({ name: '', description: '', type: 'standalone' as string, city: '', country: '', lat: '', lon: '', agent_url: '', agent_token: '', active: true, enabled_cmds: ['ping', 'traceroute', 'bgp_route'] as string[] })
  const [tokenCopied, setTokenCopied] = useState(false)

  const allCmds = [
    { value: 'ping', labelKey: 'cmd.ping' },
    { value: 'traceroute', labelKey: 'cmd.traceroute' },
    { value: 'bgp_route', labelKey: 'cmd.bgp_route' },
  ]

  function toggleCmd(cmd: string) {
    setForm(f => ({
      ...f,
      enabled_cmds: f.enabled_cmds.includes(cmd)
        ? f.enabled_cmds.filter(c => c !== cmd)
        : [...f.enabled_cmds, cmd],
    }))
  }

  const load = () => api.get<Node[]>('/admin/nodes').then(setNodes).catch(() => {})
  useEffect(() => { load() }, [])

  function openCreate() {
    setEditNode(null)
    setTokenCopied(false)
    setForm({ name: '', description: '', type: 'standalone', city: '', country: '', lat: '', lon: '', agent_url: '', agent_token: '', active: true, enabled_cmds: ['ping', 'traceroute', 'bgp_route'] })
    setDialogOpen(true)
  }

  function openEdit(node: Node) {
    setEditNode(node)
    setTokenCopied(false)
    setForm({ name: node.name, description: node.description, type: node.type, city: node.city || '', country: node.country || '', lat: node.lat != null ? String(node.lat) : '', lon: node.lon != null ? String(node.lon) : '', agent_url: node.agent_url, agent_token: node.agent_token || '', active: node.active, enabled_cmds: normalizeEnabledCmds(node.enabled_cmds ?? ['ping', 'traceroute', 'bgp_route']) })
    setDialogOpen(true)
  }

  async function handleSave() {
    setSaving(true)
    try {
      const body: Record<string, unknown> = { name: form.name, description: form.description, type: form.type, city: form.city, country: form.country, agent_url: form.agent_url, agent_token: form.agent_token, active: form.active, enabled_cmds: normalizeEnabledCmds(form.enabled_cmds) }
      const lat = parseFloat(form.lat)
      const lon = parseFloat(form.lon)
      if (!isNaN(lat)) body.lat = lat
      if (!isNaN(lon)) body.lon = lon
      if (editNode) await api.put(`/admin/nodes/${editNode.id}`, body)
      else await api.post('/admin/nodes', body)
      setDialogOpen(false)
      load()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  async function handleSetDefault(id: number) {
    await api.patch(`/admin/nodes/${id}/default`)
    load()
  }

  function generateAgentToken(): string {
    const bytes = new Uint8Array(16)
    crypto.getRandomValues(bytes)
    return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
  }

  async function handleGenerateToken() {
    const token = generateAgentToken()
    setForm(f => ({ ...f, agent_token: token }))
    try {
      await navigator.clipboard.writeText(token)
      setTokenCopied(true)
      setTimeout(() => setTokenCopied(false), 2000)
    } catch {
      setTokenCopied(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.delete_node_confirm'))) return
    await api.delete(`/admin/nodes/${id}`)
    load()
  }

  async function handleTest(id: number) {
    try {
      const res = await api.post<{ status: string; message: string }>(`/admin/nodes/${id}/test`)
      alert(res.status === 'ok' ? t('admin.connection_ok') : `${t('admin.connection_error')}: ${res.message}`)
    } catch (err) {
      alert(err instanceof Error ? err.message : t('admin.connection_error'))
    }
  }

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.nodes')} eyebrow={t('admin.title')}>
        <Button onClick={openCreate}><Plus className="w-4 h-4 mr-1" /> {t('admin.add_node')}</Button>
      </PageHeader>
      <AdminPanel padded={false}>
        <div className="admin-table-wrap">
      <Table containerClassName="admin-table-inner">
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.name')}</TableHead>
            <TableHead className="hidden md:table-cell">{t('admin.location')}</TableHead>
            <TableHead>{t('admin.type')}</TableHead>
            <TableHead>{t('admin.status')}</TableHead>
            <TableHead className="hidden md:table-cell">{t('admin.commands')}</TableHead>
            <TableHead>{t('admin.actions')}</TableHead>
            <TableHead className="w-[4.5rem]">{t('admin.default_node')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {nodes.map(n => (
            <TableRow key={n.id}>
              <TableCell><div className="font-medium">{n.name}</div><div className="text-xs text-muted-foreground">{n.description}</div></TableCell>
              <TableCell className="hidden md:table-cell"><div className="text-sm">{n.city}{n.country ? ", " + n.country : ""}</div></TableCell>
              <TableCell><Badge variant="outline">{n.type}</Badge></TableCell>
              <TableCell><Badge variant={n.active ? 'success' : 'destructive'}>{n.active ? t('admin.active') : t('admin.inactive')}</Badge></TableCell>
              <TableCell className="hidden md:table-cell"><div className="flex gap-1 flex-wrap justify-center">{normalizeEnabledCmds(n.enabled_cmds ?? []).map(c => <Badge key={c} variant="secondary" className="text-xs">{c}</Badge>)}</div></TableCell>
              <TableCell>
                <div className="inline-flex items-center justify-center gap-1">
                  <Button variant="ghost" size="icon" onClick={() => handleTest(n.id)}><Zap className="w-4 h-4" /></Button>
                  <Button variant="ghost" size="icon" onClick={() => openEdit(n)}><Pencil className="w-4 h-4" /></Button>
                  <Button variant="ghost" size="icon" onClick={() => handleDelete(n.id)}><Trash2 className="w-4 h-4" /></Button>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex justify-center">
                  <Button
                    variant="ghost"
                    size="icon"
                    title={n.is_default ? t('admin.default_node') : t('admin.set_default_node')}
                    onClick={() => handleSetDefault(n.id)}
                    disabled={n.is_default}
                  >
                    <Star className={`w-4 h-4 ${n.is_default ? 'fill-warning-on-surface text-warning-on-surface' : 'text-muted-foreground'}`} />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
        </div>
      </AdminPanel>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader><DialogTitle>{editNode ? t('admin.edit_node') : t('admin.add_node')}</DialogTitle></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>{t('admin.name')}</Label><Input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></div>
            <div className="space-y-2"><Label>{t('admin.description')}</Label><Input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} /></div>
            <div className="flex items-end gap-4">
              <div className="space-y-2 flex-1">
                <Label>{t('admin.type')}</Label>
                <Select value={form.type} onValueChange={v => setForm({ ...form, type: v })}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="standalone">{t('admin.standalone')}</SelectItem>
                    <SelectItem value="lg_node">{t('admin.lg_node')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-2 pb-1"><Switch checked={form.active} onCheckedChange={v => setForm({ ...form, active: v })} /><Label>{t('admin.active')}</Label></div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2"><Label>{t('admin.city')}</Label><Input value={form.city} onChange={e => setForm({ ...form, city: e.target.value })} placeholder="Istanbul" /></div>
              <div className="space-y-2"><Label>{t('admin.country')}</Label><Input value={form.country} onChange={e => setForm({ ...form, country: e.target.value })} placeholder="TR" /></div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2"><Label>{t('admin.latitude')}</Label><Input value={form.lat} onChange={e => setForm({ ...form, lat: e.target.value })} placeholder="41.0082" /></div>
              <div className="space-y-2"><Label>{t('admin.longitude')}</Label><Input value={form.lon} onChange={e => setForm({ ...form, lon: e.target.value })} placeholder="28.9784" /></div>
            </div>
            {form.type === 'standalone' && (
              <div className="space-y-2">
                <Label>{t('admin.agent_token')}</Label>
                <div className="flex gap-2">
                  <Input
                    className="flex-1 font-mono text-sm"
                    type="text"
                    value={form.agent_token}
                    onChange={e => setForm({ ...form, agent_token: e.target.value })}
                    placeholder={editNode ? t('admin.agent_token_placeholder') : ''}
                    autoComplete="off"
                    spellCheck={false}
                  />
                  <Button type="button" variant="outline" className="shrink-0" onClick={handleGenerateToken}>
                    {t('admin.agent_token_generate')}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">{t('admin.standalone_token_help')}</p>
                {tokenCopied && <p className="text-xs text-muted-foreground">{t('admin.agent_token_copied')}</p>}
              </div>
            )}
            {form.type === 'lg_node' && (
              <>
                <div className="space-y-2"><Label>{t('admin.agent_url')}</Label><Input value={form.agent_url} onChange={e => setForm({ ...form, agent_url: e.target.value })} placeholder="http://host:9090" /></div>
                <div className="space-y-2">
                  <Label>{t('admin.agent_token')}</Label>
                  <div className="flex gap-2">
                    <Input
                      className="flex-1 font-mono text-sm"
                      type="text"
                      value={form.agent_token}
                      onChange={e => setForm({ ...form, agent_token: e.target.value })}
                      placeholder={editNode ? t('admin.agent_token_placeholder') : ''}
                      autoComplete="off"
                      spellCheck={false}
                    />
                    <Button type="button" variant="outline" className="shrink-0" onClick={handleGenerateToken}>
                      {t('admin.agent_token_generate')}
                    </Button>
                  </div>
                  {tokenCopied && <p className="text-xs text-muted-foreground">{t('admin.agent_token_copied')}</p>}
                </div>
              </>
            )}
            <div className="space-y-2">
              <Label>{t('admin.enabled_commands')}</Label>
              <div className="flex flex-wrap gap-2">
                {allCmds.map(cmd => (
                  <label key={cmd.value} className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border text-sm cursor-pointer transition-colors ${form.enabled_cmds.includes(cmd.value) ? 'bg-primary text-primary-foreground border-primary' : 'bg-background border-input hover:bg-accent'}`}>
                    <input type="checkbox" className="sr-only" checked={form.enabled_cmds.includes(cmd.value)} onChange={() => toggleCmd(cmd.value)} />
                    {t(cmd.labelKey)}
                  </label>
                ))}
              </div>
            </div>
          </div>
          <DialogFooter><Button onClick={handleSave} disabled={saving}>{t('admin.save')}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
