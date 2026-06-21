import { useEffect, useState } from 'react'
import { Plus, Pencil, Trash2 } from 'lucide-react'
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
import type { Node, QuickQuery } from '@/types/domain'

const commandOptions = [
  { value: 'ping', labelKey: 'cmd.ping' },
  { value: 'traceroute', labelKey: 'cmd.traceroute' },
  { value: 'bgp_route', labelKey: 'cmd.bgp_route' },
] as const

const defaultNodeValue = '__default__'

export function QuickQueriesPage() {
  const { t } = useI18n()
  const [items, setItems] = useState<QuickQuery[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editItem, setEditItem] = useState<QuickQuery | null>(null)
  const [form, setForm] = useState({ command: 'ping', name: '', target: '', node_id: defaultNodeValue, active: true })

  const load = () => {
    api.get<QuickQuery[]>('/admin/quick-queries').then(setItems).catch(() => {})
    api.get<Node[]>('/admin/nodes').then(setNodes).catch(() => setNodes([]))
  }
  useEffect(() => { load() }, [])

  function nodeName(nodeId?: number | null): string {
    if (!nodeId) return t('admin.quick_query_node_default')
    const node = nodes.find(n => n.id === nodeId)
    return node ? node.name : String(nodeId)
  }

  function openCreate() {
    setEditItem(null)
    setForm({ command: 'ping', name: '', target: '', node_id: defaultNodeValue, active: true })
    setDialogOpen(true)
  }

  function openEdit(item: QuickQuery) {
    setEditItem(item)
    setForm({
      command: item.command,
      name: item.name,
      target: item.target,
      node_id: item.node_id ? String(item.node_id) : defaultNodeValue,
      active: item.active,
    })
    setDialogOpen(true)
  }

  async function handleSave() {
    setSaving(true)
    try {
      const body = {
        command: form.command,
        name: form.name,
        target: form.target,
        active: form.active,
        node_id: form.node_id === defaultNodeValue ? null : Number(form.node_id),
      }
      if (editItem) await api.put(`/admin/quick-queries/${editItem.id}`, body)
      else await api.post('/admin/quick-queries', body)
      setDialogOpen(false)
      load()
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.delete_quick_query_confirm'))) return
    await api.delete(`/admin/quick-queries/${id}`)
    load()
  }

  async function handleToggle(id: number) {
    await api.patch(`/admin/quick-queries/${id}/toggle`)
    load()
  }

  function commandLabel(command: string) {
    const key = commandOptions.find(c => c.value === command)?.labelKey
    return key ? t(key) : command
  }

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.quick_queries')} eyebrow={t('admin.title')}>
        <Button onClick={openCreate}><Plus className="w-4 h-4 mr-1" /> {t('admin.add_quick_query')}</Button>
      </PageHeader>
      <AdminPanel padded={false}>
        <div className="admin-table-wrap">
          <Table containerClassName="admin-table-inner">
            <TableHeader>
              <TableRow>
                <TableHead>{t('admin.quick_query_name')}</TableHead>
                <TableHead>{t('admin.quick_query_type')}</TableHead>
                <TableHead>{t('admin.quick_query_node')}</TableHead>
                <TableHead>{t('admin.quick_query_target')}</TableHead>
                <TableHead>{t('admin.actions')}</TableHead>
                <TableHead className="w-[4.5rem]">{t('admin.active')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map(item => (
                <TableRow key={item.id}>
                  <TableCell className="font-medium">{item.name}</TableCell>
                  <TableCell><Badge variant="outline">{commandLabel(item.command)}</Badge></TableCell>
                  <TableCell><Badge variant="secondary">{nodeName(item.node_id)}</Badge></TableCell>
                  <TableCell className="font-data text-sm">{item.target}</TableCell>
                  <TableCell>
                    <div className="inline-flex items-center justify-center gap-1">
                      <Button variant="ghost" size="icon" onClick={() => openEdit(item)}><Pencil className="w-4 h-4" /></Button>
                      <Button variant="ghost" size="icon" onClick={() => handleDelete(item.id)}><Trash2 className="w-4 h-4" /></Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-center">
                      <Switch
                        checked={item.active}
                        onCheckedChange={() => handleToggle(item.id)}
                        aria-label={item.active ? t('admin.active') : t('admin.inactive')}
                      />
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </AdminPanel>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editItem ? t('admin.edit_quick_query') : t('admin.add_quick_query')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t('admin.quick_query_name')}</Label>
              <Input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} placeholder="Cloudflare" />
            </div>
            <div className="space-y-2">
              <Label>{t('admin.quick_query_type')}</Label>
              <Select value={form.command} onValueChange={v => setForm({ ...form, command: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {commandOptions.map(c => (
                    <SelectItem key={c.value} value={c.value}>{t(c.labelKey)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t('admin.quick_query_node')}</Label>
              <Select value={form.node_id} onValueChange={v => setForm({ ...form, node_id: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value={defaultNodeValue}>{t('admin.quick_query_node_default')}</SelectItem>
                  {nodes.map(n => (
                    <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t('admin.quick_query_node_hint')}</p>
            </div>
            <div className="space-y-2">
              <Label>{t('admin.quick_query_target')}</Label>
              <Input value={form.target} onChange={e => setForm({ ...form, target: e.target.value })} placeholder="1.1.1.1" className="font-data" />
            </div>
            <div className="flex items-center gap-3">
              <Switch
                id="quick-query-active"
                checked={form.active}
                onCheckedChange={v => setForm({ ...form, active: v })}
                aria-label={form.active ? t('admin.active') : t('admin.inactive')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button onClick={handleSave} disabled={saving || !form.name.trim() || !form.target.trim()}>
              {t('admin.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
