import { useState } from 'react'
import { Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { AdminPanel } from '@/components/admin/admin-panel'
import { QueryErrorAlert } from '@/components/results/query-error-alert'
import { api, ApiError } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { BGPPathDetail, Node } from '@/types/domain'

const ALL_NODES = 'all'

interface Props {
  nodes: Node[]
}

/**
 * Shows the paths behind a prefix exactly as they reached us — ADD-PATH identifier, every
 * attribute, no best-path selection — so what HopStat displays can be checked against what
 * the router advertised.
 */
export function BGPPathLookup({ nodes }: Props) {
  const { t } = useI18n()
  const [prefix, setPrefix] = useState('')
  const [nodeId, setNodeId] = useState(ALL_NODES)
  const [paths, setPaths] = useState<BGPPathDetail[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function lookup(e: React.FormEvent) {
    e.preventDefault()
    const target = prefix.trim()
    if (!target || loading) return

    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ prefix: target })
      if (nodeId !== ALL_NODES) params.set('node_id', nodeId)
      setPaths(await api.get<BGPPathDetail[]>(`/admin/bgp/paths?${params.toString()}`))
    } catch (err: unknown) {
      setPaths(null)
      setError(err instanceof ApiError || err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <AdminPanel>
      <form onSubmit={lookup} className="flex flex-wrap items-end gap-3">
        <div className="min-w-[14rem] flex-1">
          <Label htmlFor="bgp-lookup-prefix">{t('admin.bgp_lookup_prefix')}</Label>
          <Input
            id="bgp-lookup-prefix"
            value={prefix}
            onChange={e => setPrefix(e.target.value)}
            placeholder="8.8.8.0/24"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
          />
        </div>

        <div className="w-48">
          <Label>{t('admin.bgp_lookup_node')}</Label>
          <Select value={nodeId} onValueChange={setNodeId}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_NODES}>{t('admin.bgp_lookup_all_nodes')}</SelectItem>
              {nodes.map(n => (
                <SelectItem key={n.id} value={String(n.id)}>{n.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Button type="submit" disabled={!prefix.trim() || loading}>
          <Search className="mr-1 h-4 w-4" /> {t('admin.bgp_lookup_run')}
        </Button>
      </form>

      {error && <QueryErrorAlert message={error} className="mt-4" />}

      {paths && paths.length === 0 && (
        <p className="mt-4 text-sm text-muted-foreground">{t('admin.bgp_lookup_empty')}</p>
      )}

      {paths && paths.length > 0 && (
        <div className="mt-4 space-y-3">
          {paths.map((path, i) => (
            <div key={`${path.neighbor_ip}-${path.identifier}-${i}`} className="rounded-lg border border-border p-3">
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-xs">
                <span className="font-semibold">{path.prefix}</span>
                <span>path-id: {path.identifier}</span>
                <span>{t('admin.bgp_lookup_from')}: {path.node_name || path.neighbor_ip}</span>
                {path.best && <span className="text-brand-accent">{t('result.best')}</span>}
                <span className="text-muted-foreground">{path.age}</span>
              </div>

              <div className="mt-2 grid gap-x-6 gap-y-1 font-mono text-xs text-muted-foreground sm:grid-cols-2">
                <span>AS path: {path.as_path || '—'}</span>
                <span>next hop: {path.next_hop || '—'}</span>
                <span>origin: {path.origin || '—'}</span>
                <span>local pref: {path.local_pref || '—'}</span>
                <span>MED: {path.med || '—'}</span>
                <span>neighbor: {path.neighbor_ip || '—'}</span>
              </div>

              {path.attributes.length > 0 && (
                <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground">
                  {path.attributes.join('\n')}
                </pre>
              )}
            </div>
          ))}
        </div>
      )}
    </AdminPanel>
  )
}
