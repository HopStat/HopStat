import { useEffect, useState } from 'react'
import { Plus, Pencil, Trash2, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { BGPNeighbor, Node } from '@/types/domain'

const statusVariant = (state: string): 'success' | 'warning' | 'secondary' => {
  if (state === 'established') return 'success'
  if (state === 'idle') return 'secondary'
  return 'warning'
}

export function BGPNeighborsPage() {
  const { t } = useI18n()
  const [neighbors, setNeighbors] = useState<BGPNeighbor[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editItem, setEditItem] = useState<BGPNeighbor | null>(null)
  const [form, setForm] = useState({
    node_id: '' as string,
    local_as: '',
    remote_as: '',
    peering_ip: '',
    neighbor_ip: '',
    ipv6_peering_ip: '',
    ipv6_neighbor_ip: '',
    multihop: false,
  })

  const load = () => {
    api.get<BGPNeighbor[]>('/admin/bgp-neighbors').then(setNeighbors).catch(() => {})
    api.get<Node[]>('/admin/nodes').then(setNodes).catch(() => {})
  }
  useEffect(() => { load() }, [])

  function openCreate() {
    setEditItem(null)
    setForm({ node_id: '', local_as: '', remote_as: '', peering_ip: '', neighbor_ip: '', ipv6_peering_ip: '', ipv6_neighbor_ip: '', multihop: false })
    setDialogOpen(true)
  }

  function openEdit(n: BGPNeighbor) {
    setEditItem(n)
    setForm({
      node_id: String(n.node_id),
      local_as: String(n.local_as),
      remote_as: String(n.remote_as),
      peering_ip: n.peering_ip,
      neighbor_ip: n.neighbor_ip,
      ipv6_peering_ip: n.ipv6_peering_ip ?? '',
      ipv6_neighbor_ip: n.ipv6_neighbor_ip ?? '',
      multihop: n.multihop,
    })
    setDialogOpen(true)
  }

  async function handleSave() {
    const body = {
      node_id: Number(form.node_id),
      local_as: Number(form.local_as),
      remote_as: Number(form.remote_as),
      peering_ip: form.peering_ip,
      neighbor_ip: form.neighbor_ip,
      ipv6_peering_ip: form.ipv6_peering_ip,
      ipv6_neighbor_ip: form.ipv6_neighbor_ip,
      multihop: form.multihop,
    }
    if (editItem) await api.put(`/admin/bgp-neighbors/${editItem.id}`, body)
    else await api.post('/admin/bgp-neighbors', body)
    setDialogOpen(false)
    load()
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.bgp_delete_confirm'))) return
    await api.delete(`/admin/bgp-neighbors/${id}`)
    load()
  }

  function nodeName(nodeId: number): string {
    const n = nodes.find(n => n.id === nodeId)
    return n ? n.name : String(nodeId)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('admin.bgp_neighbors')}</h1>
        <div className="flex gap-2">
          <Button variant="outline" onClick={load}><RefreshCw className="w-4 h-4 mr-1" /> {t('admin.bgp_refresh')}</Button>
          <Button onClick={openCreate}><Plus className="w-4 h-4 mr-1" /> {t('admin.bgp_add')}</Button>
        </div>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.bgp_node')}</TableHead>
            <TableHead>{t('admin.bgp_local_as')}</TableHead>
            <TableHead>{t('admin.bgp_remote_as')}</TableHead>
            <TableHead>{t('admin.bgp_peering_ip')}</TableHead>
            <TableHead>{t('admin.bgp_neighbor_ip')}</TableHead>
            <TableHead>{t('admin.bgp_multihop')}</TableHead>
            <TableHead>{t('admin.status')}</TableHead>
            <TableHead className="text-right">{t('admin.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {neighbors.map(n => (
            <TableRow key={n.id}>
              <TableCell><Badge variant="outline">{nodeName(n.node_id)}</Badge></TableCell>
              <TableCell>{n.local_as}</TableCell>
              <TableCell>{n.remote_as}</TableCell>
              <TableCell className="font-mono text-sm">{n.peering_ip}</TableCell>
              <TableCell className="font-mono text-sm">{n.neighbor_ip}</TableCell>
              <TableCell>{n.multihop ? t('admin.yes') : t('admin.no')}</TableCell>
              <TableCell><Badge variant={statusVariant(n.status)}>{n.status || 'idle'}</Badge></TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-1">
                  <Button variant="ghost" size="icon" onClick={() => openEdit(n)}><Pencil className="w-4 h-4" /></Button>
                  <Button variant="ghost" size="icon" onClick={() => handleDelete(n.id)}><Trash2 className="w-4 h-4" /></Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
          {neighbors.length === 0 && (
            <TableRow>
              <TableCell colSpan={8} className="text-center text-muted-foreground py-8">
                {t('admin.bgp_no_neighbors')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{editItem ? t('admin.bgp_edit') : t('admin.bgp_add')}</DialogTitle></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t('admin.bgp_node')}</Label>
              <Select value={form.node_id} onValueChange={v => setForm({ ...form, node_id: v })}>
                <SelectTrigger><SelectValue placeholder={t('admin.bgp_select_node')} /></SelectTrigger>
                <SelectContent>
                  {nodes.map(n => (
                    <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2"><Label>{t('admin.bgp_local_as')}</Label><Input type="number" value={form.local_as} onChange={e => setForm({ ...form, local_as: e.target.value })} placeholder="65000" /></div>
              <div className="space-y-2"><Label>{t('admin.bgp_remote_as')}</Label><Input type="number" value={form.remote_as} onChange={e => setForm({ ...form, remote_as: e.target.value })} placeholder="174" /></div>
            </div>
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">IPv4</p>
              <div className="space-y-2"><Label>{t('admin.bgp_peering_ip')}</Label><Input value={form.peering_ip} onChange={e => setForm({ ...form, peering_ip: e.target.value })} placeholder="192.168.1.1" /></div>
              <div className="space-y-2"><Label>{t('admin.bgp_neighbor_ip')}</Label><Input value={form.neighbor_ip} onChange={e => setForm({ ...form, neighbor_ip: e.target.value })} placeholder="10.0.0.1" /></div>
            </div>
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">IPv6 <span className="normal-case font-normal">({t('admin.optional')})</span></p>
              <div className="space-y-2"><Label>{t('admin.bgp_peering_ip')}</Label><Input value={form.ipv6_peering_ip} onChange={e => setForm({ ...form, ipv6_peering_ip: e.target.value })} placeholder="2001:db8::1" /></div>
              <div className="space-y-2"><Label>{t('admin.bgp_neighbor_ip')}</Label><Input value={form.ipv6_neighbor_ip} onChange={e => setForm({ ...form, ipv6_neighbor_ip: e.target.value })} placeholder="2001:db8::2" /></div>
            </div>
            <div className="flex items-center gap-2"><Switch checked={form.multihop} onCheckedChange={v => setForm({ ...form, multihop: v })} /><Label>{t('admin.bgp_multihop')}</Label></div>
          </div>
          <DialogFooter><Button onClick={handleSave}>{t('admin.save')}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
