import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import type { TranslationKey } from '@/i18n/en'
import type { User } from '@/types'
import { formatDate } from '@/lib/format'
import { Modal } from '@/components/Modal'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { SettingsContainer } from '@/components/SettingsSaver'
import { ServerSettings } from '@/components/ServerSettings'
import { LibrarySettings } from '@/components/LibrarySettings'
import { LdapSettings } from '@/components/LdapSettings'
import { PanesSettings } from '@/components/PanesSettings'
import { IconCheck, IconEdit, IconPlus, IconTrash } from '@/components/icons'

type Perm = 'isAdmin' | 'canDownload' | 'canUpload' | 'canEdit'

const PERM_LABELS: { key: Perm; labelKey: TranslationKey }[] = [
  { key: 'isAdmin', labelKey: 'admin.permAdmin' },
  { key: 'canDownload', labelKey: 'admin.permDownload' },
  { key: 'canUpload', labelKey: 'admin.permUpload' },
  { key: 'canEdit', labelKey: 'admin.permEdit' },
]

function PermBadge({ active, label }: { active: boolean; label: string }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ${
        active ? 'bg-accent-500/15 text-accentSoft' : 'bg-ink-800 text-slate-600'
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
  const { t } = useI18n()
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
    onError: (e) => setError(e instanceof ApiError ? e.message : t('admin.failedToCreate')),
  })

  return (
    <Modal open={open} onClose={onClose} title={t('admin.createTitle')}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (form.password.length < 8) {
            setError(t('admin.passwordTooShort'))
            return
          }
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">{t('admin.username')}</label>
          <input
            className="input"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            autoFocus
            required
          />
        </div>
        <div>
          <label className="label">{t('admin.password')}</label>
          <input
            className="input"
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
          />
          <p className="mt-1.5 text-xs text-slate-500">{t('admin.atLeast8')}</p>
        </div>
        <div className="grid grid-cols-2 gap-3 rounded-xl border border-ink-700 bg-ink-900 p-4">
          {PERM_LABELS.map(({ key, labelKey }) => (
            <Checkbox
              key={key}
              label={t(labelKey)}
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
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            {t('admin.createUser')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function EditUserModal({ user, open, onClose }: { user: User; open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { t } = useI18n()
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
    onError: (e) => setError(e instanceof ApiError ? e.message : t('admin.failedToUpdate')),
  })

  return (
    <Modal open={open} onClose={onClose} title={t('admin.editTitle', { username: user.username })}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          if (form.password && form.password.length < 8) {
            setError(t('admin.passwordTooShort'))
            return
          }
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <div>
          <label className="label">{t('admin.resetPassword')}</label>
          <input
            className="input"
            type="password"
            placeholder={t('admin.leaveBlank')}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </div>
        <div className="grid grid-cols-2 gap-3 rounded-xl border border-ink-700 bg-ink-900 p-4">
          {PERM_LABELS.map(({ key, labelKey }) => (
            <Checkbox
              key={key}
              label={t(labelKey)}
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
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-primary" disabled={mutation.isPending}>
            {mutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.save')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

type TabKey = 'general' | 'library' | 'auth' | 'panes' | 'users'

export function AdminPage() {
  const queryClient = useQueryClient()
  const { user: me } = useAuth()
  const { t } = useI18n()
  const [tab, setTab] = useState<TabKey>('general')
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<User | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<User | null>(null)

  const tabs: { key: TabKey; label: string }[] = [
    { key: 'general', label: t('settings.tabGeneral') },
    { key: 'library', label: t('settings.tabLibrary') },
    { key: 'auth', label: t('settings.tabAuth') },
    { key: 'panes', label: t('settings.tabPanes') },
    { key: 'users', label: t('settings.tabUsers') },
  ]

  const { data: users, isLoading } = useQuery({ queryKey: ['admin-users'], queryFn: api.adminUsers })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmDelete(null)
    },
  })

  const cantLoginBadge = (
    <span className="inline-flex items-center rounded-full bg-red-500/15 px-2 py-0.5 text-[11px] font-medium text-red-300">
      {t('admin.cannotLogin')}
    </span>
  )
  const userActions = (u: User) => (
    <div className="flex shrink-0 justify-end gap-1">
      <button
        type="button"
        onClick={() => setEditUser(u)}
        className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-ink-700 hover:text-white"
        aria-label={t('admin.editUser')}
      >
        <IconEdit width={16} height={16} />
      </button>
      <button
        type="button"
        onClick={() => setConfirmDelete(u)}
        disabled={u.id === me?.id}
        className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-red-500/10 hover:text-red-300 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-slate-400"
        aria-label={t('admin.deleteUser')}
        title={u.id === me?.id ? t('admin.cannotDeleteSelf') : t('admin.deleteUser')}
      >
        <IconTrash width={16} height={16} />
      </button>
    </div>
  )

  const usersTable = (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm text-slate-500">{t('admin.subtitle')}</p>
        <button type="button" className="btn-primary" onClick={() => setCreateOpen(true)}>
          <IconPlus width={16} height={16} />
          <span className="hidden sm:inline">{t('admin.newUser')}</span>
        </button>
      </div>

      {isLoading ? (
        <FullPageSpinner />
      ) : (
        <>
          {/* Mobile: stacked cards (the table is too wide for a phone). */}
          <div className="space-y-2 sm:hidden">
            {users?.map((u) => (
              <div key={u.id} className="card p-4">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-ink-700 text-xs font-semibold uppercase text-accentSoft">
                      {u.username[0]}
                    </span>
                    <div className="min-w-0">
                      <p className="truncate font-medium text-slate-100">
                        {u.username}
                        {u.id === me?.id && <span className="ml-1.5 text-xs text-slate-500">{t('admin.you')}</span>}
                      </p>
                      <p className="truncate text-xs text-slate-500">
                        {u.source} · {formatDate(u.createdAt)}
                      </p>
                    </div>
                  </div>
                  {userActions(u)}
                </div>
                <div className="mt-3 flex flex-wrap gap-1">
                  {u.canLogin === false && cantLoginBadge}
                  {PERM_LABELS.map(({ key, labelKey }) => (
                    <PermBadge key={key} active={u[key]} label={t(labelKey)} />
                  ))}
                </div>
              </div>
            ))}
          </div>

          {/* Desktop: table */}
          <div className="card hidden overflow-hidden sm:block">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-ink-700 text-xs uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-3 font-medium">{t('admin.colUser')}</th>
                    <th className="px-4 py-3 font-medium">{t('admin.colPermissions')}</th>
                    <th className="px-4 py-3 font-medium">{t('admin.colSource')}</th>
                    <th className="hidden px-4 py-3 font-medium md:table-cell">{t('admin.colCreated')}</th>
                    <th className="px-4 py-3" />
                  </tr>
                </thead>
                <tbody>
                  {users?.map((u) => (
                    <tr key={u.id} className="border-b border-ink-800 last:border-0 hover:bg-ink-800/40">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-ink-700 text-xs font-semibold uppercase text-accentSoft">
                            {u.username[0]}
                          </span>
                          <span className="font-medium text-slate-100">
                            {u.username}
                            {u.id === me?.id && <span className="ml-1.5 text-xs text-slate-500">{t('admin.you')}</span>}
                          </span>
                          {u.canLogin === false && cantLoginBadge}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap gap-1">
                          {PERM_LABELS.map(({ key, labelKey }) => (
                            <PermBadge key={key} active={u[key]} label={t(labelKey)} />
                          ))}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-slate-400">{u.source}</td>
                      <td className="hidden px-4 py-3 text-slate-400 md:table-cell">{formatDate(u.createdAt)}</td>
                      <td className="px-4 py-3">{userActions(u)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  )

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-white">{t('settings.title')}</h1>
        <p className="mt-0.5 text-sm text-slate-500">{t('settings.subtitle')}</p>
      </div>

      {/* Category tabs. overflow-y-hidden stops the underline from adding a 1px
          vertical scroll (overflow-x-auto would otherwise promote overflow-y). */}
      <div className="no-scrollbar mb-6 flex gap-1 overflow-x-auto overflow-y-hidden border-b border-ink-800">
        {tabs.map((tb) => (
          <button
            key={tb.key}
            type="button"
            onClick={() => setTab(tb.key)}
            className={`relative whitespace-nowrap px-4 py-2.5 text-sm font-medium transition-colors ${
              tab === tb.key ? 'text-white' : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            {tb.label}
            {tab === tb.key && (
              <span className="absolute inset-x-2 bottom-0 h-0.5 rounded-full bg-accent-500" />
            )}
          </button>
        ))}
      </div>

      {/* General / Library / Authentication share one save bar; kept mounted so
          edits survive switching tabs. */}
      <SettingsContainer showSaveBar={tab === 'general' || tab === 'library' || tab === 'auth'}>
        <div className={tab === 'general' ? '' : 'hidden'}>
          <ServerSettings />
        </div>
        <div className={tab === 'library' ? '' : 'hidden'}>
          <LibrarySettings />
        </div>
        <div className={tab === 'auth' ? '' : 'hidden'}>
          <LdapSettings />
        </div>
      </SettingsContainer>

      {tab === 'panes' && <PanesSettings />}
      {tab === 'users' && usersTable}

      <CreateUserModal open={createOpen} onClose={() => setCreateOpen(false)} />
      {editUser && <EditUserModal user={editUser} open={!!editUser} onClose={() => setEditUser(null)} />}

      <Modal open={!!confirmDelete} onClose={() => setConfirmDelete(null)} title={t('admin.deleteTitle')}>
        <p className="text-sm text-slate-300">
          {t('admin.deleteConfirmPrefix')}
          <span className="font-medium text-white">{confirmDelete?.username}</span>
          {t('admin.deleteConfirmSuffix')}
        </p>
        {deleteMutation.isError && (
          <p className="mt-3 text-sm text-red-300">
            {(deleteMutation.error as Error)?.message ?? t('admin.failedToDeleteUser')}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" className="btn-secondary" onClick={() => setConfirmDelete(null)}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn-danger"
            onClick={() => confirmDelete && deleteMutation.mutate(confirmDelete.id)}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending && <Spinner className="h-4 w-4" />}
            {t('common.delete')}
          </button>
        </div>
      </Modal>
    </div>
  )
}
