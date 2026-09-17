import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import * as api from '@/lib/api'
import type { AuthUser, UserRole } from '@/lib/api'
import { ApiError } from '@/lib/api'
import { useAuth } from '@/components/AuthProvider'
import { useT } from '@/components/I18nProvider'
import { EmptyState } from '@/components/EmptyState'
import { ErrorState } from '@/components/ErrorState'
import { Skeleton } from '@/components/Skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/sonner'

export function UsersPage() {
  const t = useT()
  const auth = useAuth()
  const [users, setUsers] = useState<AuthUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [editUser, setEditUser] = useState<AuthUser | null>(null)
  const [deleteUser, setDeleteUser] = useState<AuthUser | null>(null)

  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  // First user on empty DB must be admin; default admin when list empty / open mode.
  const [role, setRole] = useState<UserRole>('admin')
  const [password, setPassword] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError(false)
    api
      .listUsers()
      .then(setUsers)
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (auth.canAdmin) load()
  }, [auth.canAdmin, load])

  if (!auth.canAdmin) {
    return (
      <div className="p-6" data-testid="users-forbidden">
        <ErrorState message={t('users.forbidden')} />
      </div>
    )
  }

  const resetForm = () => {
    setEmail('')
    setName('')
    setRole(users.length === 0 || auth.mode === 'open' ? 'admin' : 'viewer')
    setPassword('')
    setShowPw(false)
  }

  const onAdd = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.createUser({ email: email.trim(), name: name.trim(), role, password })
      setAddOpen(false)
      resetForm()
      // First-user bootstrap flips open → users; refresh so AppShell can send us to login.
      await auth.refresh()
      load()
      toast({ title: t('users.added'), variant: 'success' })
    } catch (err) {
      toast({
        title: err instanceof ApiError ? err.message : t('users.add_failed'),
        variant: 'destructive',
      })
    } finally {
      setBusy(false)
    }
  }

  const onEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editUser) return
    setBusy(true)
    try {
      const body: { role?: UserRole; password?: string } = { role }
      if (password) body.password = password
      await api.patchUser(editUser.id, body)
      setEditUser(null)
      resetForm()
      load()
      toast({ title: t('users.updated'), variant: 'success' })
    } catch (err) {
      toast({
        title: err instanceof ApiError ? err.message : t('users.update_failed'),
        variant: 'destructive',
      })
    } finally {
      setBusy(false)
    }
  }

  const onDelete = async () => {
    if (!deleteUser) return
    setBusy(true)
    try {
      await api.deleteUser(deleteUser.id)
      setDeleteUser(null)
      load()
      toast({ title: t('users.deleted'), variant: 'success' })
    } catch (err) {
      toast({
        title: err instanceof ApiError ? err.message : t('users.delete_failed'),
        variant: 'destructive',
      })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6 p-6">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t('users.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('users.subtitle')}</p>
        </div>
        <Button
          onClick={() => {
            resetForm()
            setAddOpen(true)
          }}
          data-testid="users-add"
        >
          {t('users.add')}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('users.table')}</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-32 w-full" />
          ) : error ? (
            <ErrorState message={t('users.load_failed')} onRetry={load} />
          ) : users.length === 0 ? (
            <EmptyState title={t('users.empty')} body={t('users.empty_body')} />
          ) : (
            <Table data-testid="users-table">
              <TableHeader>
                <TableRow>
                  <TableHead>{t('users.col.email')}</TableHead>
                  <TableHead>{t('users.col.name')}</TableHead>
                  <TableHead>{t('users.col.role')}</TableHead>
                  <TableHead>{t('users.col.2fa')}</TableHead>
                  <TableHead>{t('users.col.created')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell>{u.email}</TableCell>
                    <TableCell>{u.name || '—'}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{u.role}</Badge>
                    </TableCell>
                    <TableCell>{u.totp_enabled ? '✓' : '—'}</TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {u.created_at?.slice(0, 10)}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setEditUser(u)
                          setRole(u.role)
                          setPassword('')
                        }}
                      >
                        {t('users.edit')}
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t('users.delete')}
                        onClick={() => setDeleteUser(u)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={addOpen} onClose={() => setAddOpen(false)} title={t('users.add')}>
        <form onSubmit={onAdd} className="space-y-3" data-testid="users-add-dialog">
          <div className="space-y-1">
            <Label>{t('users.col.email')}</Label>
            <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <div className="space-y-1">
            <Label>{t('users.col.name')}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label>{t('users.col.role')}</Label>
            <Select
              value={role}
              onChange={(e) => setRole(e.target.value as UserRole)}
              data-testid="users-role-select"
            >
              <option value="admin">admin</option>
              <option value="editor">editor</option>
              <option value="viewer">viewer</option>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>{t('login.password')}</Label>
            <div className="flex gap-2">
              <Input
                type={showPw ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={10}
                required
              />
              <Button type="button" variant="outline" onClick={() => setShowPw((s) => !s)}>
                {showPw ? t('users.hide_pw') : t('users.show_pw')}
              </Button>
            </div>
          </div>
          <Button type="submit" disabled={busy}>
            {t('users.add')}
          </Button>
        </form>
      </Dialog>

      <Dialog open={!!editUser} onClose={() => setEditUser(null)} title={t('users.edit')}>
        <form onSubmit={onEdit} className="space-y-3">
          <div className="space-y-1">
            <Label>{t('users.col.role')}</Label>
            <Select value={role} onChange={(e) => setRole(e.target.value as UserRole)}>
              <option value="admin">admin</option>
              <option value="editor">editor</option>
              <option value="viewer">viewer</option>
            </Select>
          </div>
          <div className="space-y-1">
            <Label>{t('users.reset_password')}</Label>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={10}
              placeholder={t('users.password_optional')}
            />
          </div>
          <Button type="submit" disabled={busy}>
            {t('common.save')}
          </Button>
        </form>
      </Dialog>

      <Dialog
        open={!!deleteUser}
        onClose={() => setDeleteUser(null)}
        title={t('users.delete')}
        description={t('users.delete_confirm', { email: deleteUser?.email || '' })}
      >
        <div className="flex justify-end gap-2" data-testid="users-delete-confirm">
          <Button variant="ghost" onClick={() => setDeleteUser(null)}>
            {t('common.dismiss')}
          </Button>
          <Button variant="destructive" onClick={onDelete} disabled={busy}>
            {t('users.delete')}
          </Button>
        </div>
      </Dialog>
    </div>
  )
}
