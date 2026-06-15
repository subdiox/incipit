import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import type { User } from '@/types'
import { formatDate } from '@/lib/format'
import { Modal } from '@/components/Modal'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconCheck, IconEdit, IconPlus, IconTrash } from '@/components/icons'

type Perm = 'isAdmin' | 'canDownload' | 'canUpload' | 'canEdit'

const PERM_LABELS: { key: Perm; label: string }[] = [
  { key: 'isAdmin', label: 'Admin' },
  { key: 'canDownload', label: 'Download' },
  { key: 'canUpload', label: 'Upload' },
  { key: 'canEdit', label: 'Edit' },
]

function PermBadge({ active, label }: { active: boolean; label: string }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ${
        active ? 'bg-accent-500/15 text-accent-200' : 'bg-ink-800 text-slate-600'
      }`}
    >
      {active && <IconCheck width={11} height={11} />}
      {label}
    </span>
  )
}

function Checkbox({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <label className="flex items-center gap-2.5 text-sm text-slate-300">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4 rounded border-ink-600 bg-ink-800 text-accent-500 focus:ring-accent-500/40"
      />
      {label}
    </label>
  )
}

function CreateUserModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    username: '',
    password: '',
    isAdmin: false,
    canDownload: true,
    canUpload: false,
    canEdit: false,
  })
  const [error, setError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: () => api.createUser(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setForm({ username: '', password: '', isAdmin: false, canDownload: true, canUpload: false, canEdit: false })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Failed to create user.'),
  })

  return (
    <Modal open={open} onClose={onClose} title="Create user">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (form.password.length < 8) {
            setError('Password must be at least 8 characters.')
            return
          }
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">Username</label>
          <input
            className="input"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            autoFocus
            required
          />
        </div>
        <div>
          <label className="label">Password</label>
          <input
            className="input"
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
          />
          <p className="mt-1.5 text-xs text-slate-500">At least 8 characters.</p>
        </div>
        <div className="grid grid-cols-2 gap-3 rounded-xl border border-ink-700 bg-ink-900 p-4">
          {PERM_LABELS.map(({ key, label }) => (
            <Checkbox
              key={key}
              label={label}
              checked={form[key]}
              onChange={(v) => setForm({ ...form, [key]: v })}
            />
          ))}
        </div>
        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            Create user
          </button>
        </div>
      </form>
    </Modal>
  )
}

function EditUserModal({ user, open, onClose }: { user: User; open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    password: '',
    isAdmin: user.isAdmin,
    canDownload: user.canDownload,
    canUpload: user.canUpload,
    canEdit: user.canEdit,
  })
  const [error, setError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: () =>
      api.updateUser(user.id, {
        password: form.password || undefined,
        isAdmin: form.isAdmin,
        canDownload: form.canDownload,
        canUpload: form.canUpload,
        canEdit: form.canEdit,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      onClose()
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Failed to update user.'),
  })

  return (
    <Modal open={open} onClose={onClose} title={`Edit ${user.username}`}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (form.password && form.password.length < 8) {
            setError('Password must be at least 8 characters.')
            return
          }
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">Reset password (optional)</label>
          <input
            className="input"
            type="password"
            placeholder="Leave blank to keep current"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </div>
        <div className="grid grid-cols-2 gap-3 rounded-xl border border-ink-700 bg-ink-900 p-4">
          {PERM_LABELS.map(({ key, label }) => (
            <Checkbox
              key={key}
              label={label}
              checked={form[key]}
              onChange={(v) => setForm({ ...form, [key]: v })}
            />
          ))}
        </div>
        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            Save
          </button>
        </div>
      </form>
    </Modal>
  )
}

export function AdminPage() {
  const queryClient = useQueryClient()
  const { user: me } = useAuth()
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<User | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<User | null>(null)

  const { data: users, isLoading } = useQuery({ queryKey: ['admin-users'], queryFn: api.adminUsers })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmDelete(null)
    },
  })

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-white">Users</h1>
          <p className="mt-0.5 text-sm text-slate-500">Manage accounts and permissions.</p>
        </div>
        <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">New user</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : (
        <div className="card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-ink-700 text-xs uppercase tracking-wide text-slate-500">
                  <th className="px-4 py-3 font-medium">User</th>
                  <th className="px-4 py-3 font-medium">Permissions</th>
                  <th className="px-4 py-3 font-medium">Source</th>
                  <th className="hidden px-4 py-3 font-medium md:table-cell">Created</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody>
                {users?.map((u) => (
                  <tr key={u.id} className="border-b border-ink-800 last:border-0 hover:bg-ink-800/40">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-3">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-ink-700 text-xs font-semibold uppercase text-accent-300">
                          {u.username[0]}
                        </span>
                        <span className="font-medium text-slate-100">
                          {u.username}
                          {u.id === me?.id && <span className="ml-1.5 text-xs text-slate-500">(you)</span>}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {PERM_LABELS.map(({ key, label }) => (
                          <PermBadge key={key} active={u[key]} label={label} />
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-slate-400">{u.source}</td>
                    <td className="hidden px-4 py-3 text-slate-400 md:table-cell">{formatDate(u.createdAt)}</td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1">
                        <button
                          type="button"
                          onClick={() => setEditUser(u)}
                          className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-ink-700 hover:text-white"
                          aria-label="Edit user"
                        >
                          <IconEdit width={16} height={16} />
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmDelete(u)}
                          disabled={u.id === me?.id}
                          className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-red-500/10 hover:text-red-300 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-slate-400"
                          aria-label="Delete user"
                          title={u.id === me?.id ? 'You cannot delete yourself' : 'Delete user'}
                        >
                          <IconTrash width={16} height={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <CreateUserModal open={createOpen} onClose={() => setCreateOpen(false)} />
      {editUser && <EditUserModal user={editUser} open={!!editUser} onClose={() => setEditUser(null)} />}

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title="Delete user">
        <p className="text-sm text-slate-300">
          Delete user <span className="font-medium text-white">{confirmDelete?.username}</span>? This cannot be
          undone.
        </p>
        {deleteMutation.isError && (
          <p className="mt-3 text-sm text-red-300">
            {(deleteMutation.error as Error)?.message ?? 'Failed to delete user.'}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(null)}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-danger"
            onClick={() => confirmDelete && deleteMutation.mutate(confirmDelete.id)}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending && <Spinner className="h-4 w-4" />}
            Delete
          </button>
        </div>
      </Modal>
    </div>
  )
}
