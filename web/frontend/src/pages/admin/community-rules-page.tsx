import { useEffect, useState } from 'react'
import { Plus, Pencil, Trash2, ToggleLeft } from 'lucide-react'
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
import type { CommunityRule } from '@/types/domain'

const severityVariant = { reject: 'destructive' as const, warning: 'warning' as const, info: 'info' as const, success: 'success' as const }

export function CommunityRulesPage() {
  const { t } = useI18n()
  const [rules, setRules] = useState<CommunityRule[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editRule, setEditRule] = useState<CommunityRule | null>(null)
  const [form, setForm] = useState({ community: '', severity: 'info' as string, message_i18n: '', scope: 'global', active: true })

  const load = () => api.get<CommunityRule[]>('/admin/community-rules').then(setRules).catch(() => {})
  useEffect(() => { load() }, [])

  function openCreate() {
    setEditRule(null)
    setForm({ community: '', severity: 'info', message_i18n: '', scope: 'global', active: true })
    setDialogOpen(true)
  }

  function openEdit(rule: CommunityRule) {
    setEditRule(rule)
    setForm({ community: rule.community, severity: rule.severity, message_i18n: rule.message_i18n, scope: rule.scope, active: rule.active })
    setDialogOpen(true)
  }

  async function handleSave() {
    setSaving(true)
    try {
      const body = { ...form }
      if (editRule) await api.put(`/admin/community-rules/${editRule.id}`, body)
      else await api.post('/admin/community-rules', body)
      setDialogOpen(false)
      load()
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.delete_rule_confirm'))) return
    await api.delete(`/admin/community-rules/${id}`)
    load()
  }

  async function handleToggle(id: number) {
    await api.patch(`/admin/community-rules/${id}/toggle`)
    load()
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('admin.community_rules')}</h1>
        <Button onClick={openCreate}><Plus className="w-4 h-4 mr-1" /> {t('admin.add_rule')}</Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.community')}</TableHead>
            <TableHead>{t('admin.severity')}</TableHead>
            <TableHead className="hidden md:table-cell">{t('admin.message')}</TableHead>
            <TableHead>{t('admin.active')}</TableHead>
            <TableHead className="text-right">{t('admin.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rules.map(r => (
            <TableRow key={r.id}>
              <TableCell className="font-mono text-sm">{r.community}</TableCell>
              <TableCell><Badge variant={severityVariant[r.severity as keyof typeof severityVariant] ?? 'default'}>{r.severity}</Badge></TableCell>
              <TableCell className="hidden md:table-cell text-sm truncate max-w-xs">{r.message_i18n}</TableCell>
              <TableCell><Badge variant={r.active ? 'success' : 'secondary'}>{r.active ? t('admin.yes') : t('admin.no')}</Badge></TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-1">
                  <Button variant="ghost" size="icon" onClick={() => handleToggle(r.id)}><ToggleLeft className="w-4 h-4" /></Button>
                  <Button variant="ghost" size="icon" onClick={() => openEdit(r)}><Pencil className="w-4 h-4" /></Button>
                  <Button variant="ghost" size="icon" onClick={() => handleDelete(r.id)}><Trash2 className="w-4 h-4" /></Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{editRule ? t('admin.edit_rule') : t('admin.add_rule')}</DialogTitle></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>{t('admin.community')}</Label><Input value={form.community} onChange={e => setForm({ ...form, community: e.target.value })} placeholder="65000:123" /></div>
            <div className="space-y-2">
              <Label>{t('admin.severity')}</Label>
              <Select value={form.severity} onValueChange={v => setForm({ ...form, severity: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="info">{t('admin.severity_info')}</SelectItem>
                  <SelectItem value="success">{t('admin.severity_success')}</SelectItem>
                  <SelectItem value="warning">{t('admin.severity_warning')}</SelectItem>
                  <SelectItem value="reject">{t('admin.severity_reject')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2"><Label>{t('admin.message')}</Label><textarea className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm" value={form.message_i18n} onChange={e => setForm({ ...form, message_i18n: e.target.value })} /></div>
            <div className="flex items-center gap-2"><Switch checked={form.active} onCheckedChange={v => setForm({ ...form, active: v })} /><Label>{t('admin.active')}</Label></div>
          </div>
          <DialogFooter><Button onClick={handleSave} disabled={saving}>{t('admin.save')}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
