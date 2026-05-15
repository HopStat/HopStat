import { useEffect, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { User } from '@/types/domain'

export function UsersPage() {
  const { t } = useI18n()
  const [users, setUsers] = useState<User[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState({ email: '', password: '', role: 'admin' })

  const load = () => api.get<User[]>('/admin/users').then(setUsers).catch(() => {})
  useEffect(() => { load() }, [])

  async function handleCreate() {
    setSaving(true)
    try {
      await api.post('/admin/users', form)
      setDialogOpen(false)
      setForm({ email: '', password: '', role: 'admin' })
      load()
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t('admin.delete_user_confirm'))) return
    await api.delete(`/admin/users/${id}`)
    load()
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('admin.users')}</h1>
        <Button onClick={() => setDialogOpen(true)}><Plus className="w-4 h-4 mr-1" /> {t('admin.add_user')}</Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.email')}</TableHead>
            <TableHead>{t('admin.role')}</TableHead>
            <TableHead className="hidden sm:table-cell">{t('admin.created')}</TableHead>
            <TableHead className="hidden sm:table-cell">{t('admin.last_login')}</TableHead>
            <TableHead className="text-right">{t('admin.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map(u => (
            <TableRow key={u.id}>
              <TableCell className="font-medium">{u.email}</TableCell>
              <TableCell>{u.role}</TableCell>
              <TableCell className="hidden sm:table-cell text-muted-foreground text-sm">{u.created_at ? new Date(u.created_at.replace(' ', 'T')).toLocaleDateString() : '-'}</TableCell>
              <TableCell className="hidden sm:table-cell text-muted-foreground text-sm">{u.last_login ? new Date(u.last_login).toLocaleDateString() : '-'}</TableCell>
              <TableCell className="text-right">
                <Button variant="ghost" size="icon" onClick={() => handleDelete(u.id)}><Trash2 className="w-4 h-4" /></Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t('admin.add_user')}</DialogTitle></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>{t('admin.email')}</Label><Input type="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} /></div>
            <div className="space-y-2"><Label>{t('admin.password')}</Label><Input type="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} minLength={8} /></div>
            <div className="space-y-2"><Label>{t('admin.role')}</Label><Input value={form.role} onChange={e => setForm({ ...form, role: e.target.value })} /></div>
          </div>
          <DialogFooter><Button onClick={handleCreate} disabled={saving || !form.email || !form.password}>{t('admin.create')}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
