import { useEffect, useState } from 'react'
import { Plus, Pencil, Trash2, RefreshCw, Square, RotateCw, ScrollText } from 'lucide-react'
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
import { QueryErrorAlert } from '@/components/results/query-error-alert'
import { SystemAddressField } from '@/components/admin/system-address-field'
import { BGPPathLookup } from '@/components/admin/bgp-path-lookup'
import { api } from '@/lib/api-client'
import { cn } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'
import type { BGPNeighbor, BGPLogEntry, BGPConfigSettings, Node, SystemAddresses } from '@/types/domain'

const statusVariant = (state: string): 'success' | 'warning' | 'secondary' => {
  if (state === 'established') return 'success'
  if (state === 'idle') return 'secondary'
  return 'warning'
}

const BGP_STATUS_SHORT: Record<string, string> = {
  idle: 'IDL',
  connect: 'CON',
  active: 'ACT',
  open_sent: 'OSN',
  open_confirm: 'OCF',
  established: 'EST',
}

function normBGPState(state: string): string {
  return (state || 'idle').toLowerCase()
}

function formatBGPStatusShort(state: string): string {
  const key = normBGPState(state)
  return BGP_STATUS_SHORT[key] ?? key.slice(0, 3).toLocaleUpperCase('en-US')
}

function formatBGPStatusTitle(state: string, t: (key: string) => string): string {
  const key = normBGPState(state)
  const labelKey = `admin.bgp_status_${key}`
  const translated = t(labelKey)
  return translated !== labelKey ? translated : (state || 'idle')
}

const logLevelClass = (level: string): string => {
  if (level === 'error') return 'text-destructive'
  if (level === 'warn') return 'text-amber-600'
  return 'text-muted-foreground'
}

function isInternalPeer(n: BGPNeighbor, localAS: number): boolean {
  if (n.peer_type === 'internal') return true
  if (n.peer_type === 'external') return false
  return localAS > 0 && n.remote_as === localAS
}

function AddressStack({ lines }: { lines: Array<string | undefined | null> }) {
  const values = lines.map(v => v?.trim()).filter(Boolean) as string[]
  if (values.length === 0) return <span className="text-muted-foreground">—</span>

  return (
    <div className="admin-table__address-cell">
      {values.map((value, index) => (
        <div
          key={`${value}-${index}`}
          className={cn(index > 0 && 'text-muted-foreground text-[0.6875rem]')}
          title={value}
        >
          {value}
        </div>
      ))}
    </div>
  )
}

export function BGPNeighborsPage() {
  const { t } = useI18n()
  const [neighbors, setNeighbors] = useState<BGPNeighbor[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [bgpConfig, setBgpConfig] = useState<BGPConfigSettings | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [actionError, setActionError] = useState('')
  const [editItem, setEditItem] = useState<BGPNeighbor | null>(null)
  const [logsOpen, setLogsOpen] = useState(false)
  const [logsNeighbor, setLogsNeighbor] = useState<BGPNeighbor | null>(null)
  const [logs, setLogs] = useState<BGPLogEntry[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [systemAddresses, setSystemAddresses] = useState<SystemAddresses>({ ipv4: [], ipv6: [] })
  const [form, setForm] = useState({
    node_id: '' as string,
    remote_as: '',
    peering_ip: '',
    neighbor_ip: '',
    ipv6_peering_ip: '',
    ipv6_neighbor_ip: '',
    multihop: false,
    default_route_as: '',
  })

  const localAS = bgpConfig?.local_as ?? 0

  const load = () => {
    api.get<BGPNeighbor[]>('/admin/bgp-neighbors').then(setNeighbors).catch(() => {})
    api.get<Node[]>('/admin/nodes').then(setNodes).catch(() => {})
    api.get<BGPConfigSettings>('/admin/bgp/config').then(setBgpConfig).catch(() => setBgpConfig(null))
    api.get<SystemAddresses>('/admin/system/addresses').then(setSystemAddresses).catch(() => setSystemAddresses({ ipv4: [], ipv6: [] }))
  }
  useEffect(() => { load() }, [])

  useEffect(() => {
    const timer = setInterval(load, 15000)
    return () => clearInterval(timer)
  }, [])

  const loadLogs = (neighborId: number) => {
    setLogsLoading(true)
    api.get<BGPLogEntry[]>(`/admin/bgp-neighbors/${neighborId}/logs?limit=200`)
      .then(setLogs)
      .catch(() => setLogs([]))
      .finally(() => setLogsLoading(false))
  }

  useEffect(() => {
    if (!logsOpen || !logsNeighbor) return
    loadLogs(logsNeighbor.id)
    const timer = setInterval(() => loadLogs(logsNeighbor.id), 2000)
    return () => clearInterval(timer)
  }, [logsOpen, logsNeighbor?.id])

  function openCreate() {
    setEditItem(null)
    setSaveError('')
    const defaultNode = nodes.find(n => n.is_default) ?? nodes[0]
    setForm({
      node_id: defaultNode ? String(defaultNode.id) : '',
      remote_as: '',
      peering_ip: '',
      neighbor_ip: '',
      ipv6_peering_ip: '',
      ipv6_neighbor_ip: '',
      multihop: false,
      default_route_as: '',
    })
    setDialogOpen(true)
  }

  function openEdit(n: BGPNeighbor) {
    setEditItem(n)
    setSaveError('')
    setForm({
      node_id: String(n.node_id),
      remote_as: String(n.remote_as),
      peering_ip: n.peering_ip,
      neighbor_ip: n.neighbor_ip,
      ipv6_peering_ip: n.ipv6_peering_ip ?? '',
      ipv6_neighbor_ip: n.ipv6_neighbor_ip ?? '',
      multihop: n.multihop,
      default_route_as: n.default_route_as ? String(n.default_route_as) : '',
    })
    setDialogOpen(true)
  }

  function openLogs(n: BGPNeighbor) {
    setActionError('')
    setLogsNeighbor(n)
    setLogsOpen(true)
  }

  async function handleSave() {
    setSaveError('')
    if (!form.node_id) {
      setSaveError(t('admin.bgp_select_node'))
      return
    }
    if (!form.remote_as) {
      setSaveError(t('admin.bgp_as_required'))
      return
    }
    const hasV4 = form.peering_ip.trim() || form.neighbor_ip.trim()
    const hasV6 = form.ipv6_peering_ip.trim() || form.ipv6_neighbor_ip.trim()
    if (!hasV4 && !hasV6) {
      setSaveError(t('admin.bgp_address_required'))
      return
    }

    setSaving(true)
    try {
      const body = {
        node_id: Number(form.node_id),
        remote_as: Number(form.remote_as),
        peering_ip: form.peering_ip.trim(),
        neighbor_ip: form.neighbor_ip.trim(),
        ipv6_peering_ip: form.ipv6_peering_ip.trim(),
        ipv6_neighbor_ip: form.ipv6_neighbor_ip.trim(),
        multihop: form.multihop,
        default_route_as: form.default_route_as.trim() ? Number(form.default_route_as) : 0,
      }
      if (editItem) await api.put(`/admin/bgp-neighbors/${editItem.id}`, body)
      else await api.post('/admin/bgp-neighbors', body)
      setDialogOpen(false)
      load()
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : t('admin.error'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.bgp_delete_confirm'))) return
    await api.delete(`/admin/bgp-neighbors/${id}`)
    load()
  }

  async function handleStop(n: BGPNeighbor) {
    if (!confirm(t('admin.bgp_stop_confirm'))) return
    setActionError('')
    try {
      await api.post(`/admin/bgp-neighbors/${n.id}/stop`, {})
      load()
      if (logsOpen && logsNeighbor?.id === n.id) loadLogs(n.id)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('admin.bgp_action_failed'))
    }
  }

  async function handleRestart(n: BGPNeighbor) {
    if (!confirm(t('admin.bgp_restart_confirm'))) return
    setActionError('')
    try {
      await api.post(`/admin/bgp-neighbors/${n.id}/restart`, {})
      load()
      if (logsOpen && logsNeighbor?.id === n.id) loadLogs(n.id)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('admin.bgp_action_failed'))
    }
  }

  function nodeName(nodeId: number): string {
    const n = nodes.find(n => n.id === nodeId)
    return n ? n.name : String(nodeId)
  }

  function formatLogTime(ts: string): string {
    const d = new Date(ts)
    return Number.isNaN(d.getTime()) ? ts : d.toLocaleTimeString()
  }

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.bgp_neighbors')} eyebrow={t('admin.title')}>
        <div className="flex gap-2">
          <Button variant="outline" onClick={load}><RefreshCw className="w-4 h-4 mr-1" /> {t('admin.bgp_refresh')}</Button>
          <Button onClick={openCreate}><Plus className="w-4 h-4 mr-1" /> {t('admin.bgp_add')}</Button>
        </div>
      </PageHeader>

      {actionError && <QueryErrorAlert message={actionError} className="mb-4" />}

      {bgpConfig?.status === 'not_configured' && (
        <p className="text-sm text-amber-600">{t('admin.bgp_local_as_missing')}</p>
      )}
      {bgpConfig?.status === 'restart_required' && (
        <p className="text-sm text-amber-600">{t('admin.bgp_local_as_restart')}</p>
      )}
      {bgpConfig && bgpConfig.local_as > 0 && (
        <p className="text-sm text-muted-foreground">
          {t('admin.bgp_local_as')}: <span className="font-mono">{bgpConfig.local_as}</span>
          {bgpConfig.router_id ? <> · router-id: <span className="font-mono">{bgpConfig.router_id}</span></> : null}
          {bgpConfig.add_path_receive !== false ? <> · {t('admin.bgp_add_path_receive')}</> : null}
        </p>
      )}
      {bgpConfig?.add_path_receive !== false && (
        <p className="text-xs text-muted-foreground">{t('admin.bgp_add_path_hint')}</p>
      )}

      <BGPPathLookup nodes={nodes} />

      <AdminPanel padded={false}>
        <div className="admin-table-wrap admin-table-wrap--wide">
      <Table containerClassName="admin-table-inner" className="admin-table--bgp">
        <TableHeader>
          <TableRow>
            <TableHead className="admin-table--bgp__col-node" title={t('admin.bgp_node')}>{t('admin.bgp_col_node')}</TableHead>
            <TableHead className="admin-table--bgp__col-type" title={t('admin.bgp_peer_type')}>{t('admin.bgp_col_peer_type')}</TableHead>
            <TableHead className="admin-table--bgp__col-status" title={t('admin.status')}>{t('admin.bgp_col_status')}</TableHead>
            <TableHead className="admin-table--bgp__col-routes" title={t('admin.bgp_routes_received')}>{t('admin.bgp_col_routes')}</TableHead>
            <TableHead className="admin-table--bgp__col-as" title={t('admin.bgp_remote_as')}>{t('admin.bgp_col_remote_as')}</TableHead>
            <TableHead className="admin-table--bgp__col-local" title={t('admin.bgp_peering_ip')}>{t('admin.bgp_col_local')}</TableHead>
            <TableHead className="admin-table--bgp__col-peer" title={t('admin.bgp_neighbor_ip')}>{t('admin.bgp_col_peer')}</TableHead>
            <TableHead className="admin-table--bgp__col-mh" title={t('admin.bgp_multihop')}>{t('admin.bgp_col_multihop')}</TableHead>
            <TableHead className="admin-table--bgp__col-def" title={t('admin.default_route_as')}>{t('admin.bgp_col_default_as')}</TableHead>
            <TableHead className="admin-table--bgp__col-actions admin-table__sticky-end" title={t('admin.actions')}>{t('admin.bgp_col_actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {neighbors.map(n => (
            <TableRow key={n.id}>
              <TableCell className="admin-table--bgp__col-node admin-table--bgp__center">
                <Badge variant="outline" className="max-w-full truncate font-normal inline-block" title={nodeName(n.node_id)}>
                  {nodeName(n.node_id)}
                </Badge>
              </TableCell>
              <TableCell className="admin-table--bgp__col-type admin-table--bgp__center font-data text-sm tracking-wide">
                {(isInternalPeer(n, localAS) ? t('admin.bgp_peer_internal') : t('admin.bgp_peer_external')).toLocaleUpperCase('en-US')}
              </TableCell>
              <TableCell className="admin-table--bgp__col-status admin-table--bgp__center">
                <Badge
                  variant={statusVariant(n.status)}
                  className="admin-table__status admin-table__status--compact font-data"
                  title={formatBGPStatusTitle(n.status, t)}
                >
                  {formatBGPStatusShort(n.status)}
                </Badge>
              </TableCell>
              <TableCell className="admin-table--bgp__col-routes admin-table--bgp__center font-data text-sm tabular-nums">
                {n.status === 'established' ? (n.prefixes_received ?? 0).toLocaleString() : '—'}
              </TableCell>
              <TableCell className="admin-table--bgp__col-as admin-table--bgp__center font-data text-sm tabular-nums">{n.remote_as}</TableCell>
              <TableCell className="admin-table--bgp__col-local admin-table--bgp__address admin-table--bgp__center">
                <AddressStack lines={[n.peering_ip, n.ipv6_peering_ip]} />
              </TableCell>
              <TableCell className="admin-table--bgp__col-peer admin-table--bgp__address admin-table--bgp__center">
                <AddressStack lines={[n.neighbor_ip, n.ipv6_neighbor_ip]} />
              </TableCell>
              <TableCell className="admin-table--bgp__col-mh admin-table--bgp__center">{n.multihop ? t('admin.yes_short') : t('admin.no_short')}</TableCell>
              <TableCell className="admin-table--bgp__col-def admin-table--bgp__center font-data text-sm tabular-nums">
                {n.default_route_as ? n.default_route_as : '—'}
              </TableCell>
              <TableCell className="admin-table--bgp__col-actions admin-table__sticky-end">
                <div className="admin-table__actions">
                  <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" title={t('admin.bgp_stop')} onClick={() => handleStop(n)}><Square className="w-3 h-3" /></Button>
                  <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" title={t('admin.bgp_restart')} onClick={() => handleRestart(n)}><RotateCw className="w-3 h-3" /></Button>
                  <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" title={t('admin.bgp_logs')} onClick={() => openLogs(n)}><ScrollText className="w-3 h-3" /></Button>
                  <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" title={t('admin.edit')} onClick={() => openEdit(n)}><Pencil className="w-3 h-3" /></Button>
                  <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" title={t('admin.delete')} onClick={() => handleDelete(n.id)}><Trash2 className="w-3 h-3" /></Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
          {neighbors.length === 0 && (
            <TableRow>
              <TableCell colSpan={10} className="text-center text-muted-foreground py-8">
                {t('admin.bgp_no_neighbors')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
        </div>
      </AdminPanel>

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
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-2"><Label>{t('admin.bgp_remote_as')}</Label><Input type="number" value={form.remote_as} onChange={e => setForm({ ...form, remote_as: e.target.value })} placeholder="174" /></div>
              <div className="space-y-2">
                <Label>{t('admin.default_route_as')}</Label>
                <Input
                  type="number"
                  min={1}
                  value={form.default_route_as}
                  onChange={e => setForm({ ...form, default_route_as: e.target.value })}
                  placeholder="6453"
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">{t('admin.default_route_as_hint')}</p>
            <p className="text-xs text-muted-foreground">{t('admin.bgp_local_as_config_hint')}</p>
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">IPv4</p>
              <div className="space-y-2">
                <Label>{t('admin.bgp_peering_ip')}</Label>
                <SystemAddressField
                  key={`peering-v4-${editItem?.id ?? 'new'}`}
                  value={form.peering_ip}
                  options={systemAddresses.ipv4}
                  onChange={peering_ip => setForm({ ...form, peering_ip })}
                  family="ipv4"
                />
              </div>
              <div className="space-y-2"><Label>{t('admin.bgp_neighbor_ip')}</Label><Input value={form.neighbor_ip} onChange={e => setForm({ ...form, neighbor_ip: e.target.value })} placeholder="10.0.0.1" className="font-data" /></div>
            </div>
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">IPv6 <span className="normal-case font-normal">({t('admin.optional')})</span></p>
              <div className="space-y-2">
                <Label>{t('admin.bgp_peering_ip')}</Label>
                <SystemAddressField
                  key={`peering-v6-${editItem?.id ?? 'new'}`}
                  value={form.ipv6_peering_ip}
                  options={systemAddresses.ipv6}
                  onChange={ipv6_peering_ip => setForm({ ...form, ipv6_peering_ip })}
                  family="ipv6"
                />
              </div>
              <div className="space-y-2"><Label>{t('admin.bgp_neighbor_ip')}</Label><Input value={form.ipv6_neighbor_ip} onChange={e => setForm({ ...form, ipv6_neighbor_ip: e.target.value })} placeholder="2001:db8::2" className="font-data" /></div>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                checked={form.multihop}
                onCheckedChange={v => setForm({ ...form, multihop: v })}
              />
              <Label>{t('admin.bgp_multihop')}</Label>
            </div>
            <p className="text-xs text-muted-foreground">{t('admin.bgp_multihop_hint')}</p>
          </div>
          <DialogFooter className="flex-col items-stretch gap-2 sm:flex-col">
            {saveError && <QueryErrorAlert message={saveError} />}
            <Button onClick={handleSave} disabled={saving}>{t('admin.save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={logsOpen} onOpenChange={setLogsOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t('admin.bgp_logs_title')}
              {logsNeighbor && (
                <span className="ml-2 text-sm font-normal text-muted-foreground">
                  {nodeName(logsNeighbor.node_id)} · {logsNeighbor.neighbor_ip || logsNeighbor.ipv6_neighbor_ip}
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          <div className="rounded-md border bg-muted/30 p-3 h-80 overflow-y-auto font-mono text-xs">
            {logsLoading && logs.length === 0 ? (
              <p className="text-muted-foreground">{t('admin.loading')}</p>
            ) : logs.length === 0 ? (
              <p className="text-muted-foreground">{t('admin.bgp_logs_empty')}</p>
            ) : (
              <div className="space-y-1">
                {logs.map((entry, idx) => (
                  <div key={`${entry.timestamp}-${idx}`} className="flex gap-2">
                    <span className="text-muted-foreground shrink-0">{formatLogTime(entry.timestamp)}</span>
                    <span className={`shrink-0 w-10 ${logLevelClass(entry.level)}`}>{entry.level.toLocaleUpperCase('en-US')}</span>
                    <span className="break-all">
                      {entry.message}
                      {entry.address ? <span className="text-muted-foreground"> ({entry.address})</span> : null}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
          <DialogFooter className="gap-2 sm:justify-between">
            <div className="flex gap-2">
              {logsNeighbor && (
                <>
                  <Button variant="outline" size="sm" onClick={() => handleStop(logsNeighbor)}>{t('admin.bgp_stop')}</Button>
                  <Button variant="outline" size="sm" onClick={() => handleRestart(logsNeighbor)}>{t('admin.bgp_restart')}</Button>
                </>
              )}
            </div>
            <Button variant="outline" size="sm" onClick={() => logsNeighbor && loadLogs(logsNeighbor.id)}>
              <RefreshCw className="w-4 h-4 mr-1" /> {t('admin.bgp_refresh')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
