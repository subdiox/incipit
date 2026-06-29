import { useEffect, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { LdapSettings as LdapSettingsType } from '@/types'
import { Spinner } from './Spinner'

type Msg = { type: 'success' | 'error'; text: string } | null

type FormState = {
  enabled: boolean
  url: string
  startTLS: boolean
  bindDN: string
  baseDN: string
  userFilter: string
  usernameAttribute: string
  adminGroupDN: string
}

const EMPTY: FormState = {
  enabled: false,
  url: '',
  startTLS: false,
  bindDN: '',
  baseDN: '',
  userFilter: '(uid=%s)',
  usernameAttribute: 'uid',
  adminGroupDN: '',
}

export function LdapSettings() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['admin-ldap'], queryFn: api.ldap })

  const [form, setForm] = useState<FormState>(EMPTY)
  const [password, setPassword] = useState('')
  const [passwordSet, setPasswordSet] = useState(false)
  const [msg, setMsg] = useState<Msg>(null)

  useEffect(() => {
    if (!data) return
    setForm({
      enabled: data.enabled,
      url: data.url,
      startTLS: data.startTLS,
      bindDN: data.bindDN,
      baseDN: data.baseDN,
      userFilter: data.userFilter,
      usernameAttribute: data.usernameAttribute,
      adminGroupDN: data.adminGroupDN,
    })
    setPasswordSet(data.bindPasswordSet)
  }, [data])

  const apply = (next: LdapSettingsType) => {
    qc.setQueryData(['admin-ldap'], next)
    setPasswordSet(next.bindPasswordSet)
    setPassword('')
  }

  const saveM = useMutation({
    mutationFn: () => api.updateLdap({ ...form, bindPassword: password || undefined }),
    onSuccess: (next) => {
      apply(next)
      setMsg({ type: 'success', text: t('ldap.saved') })
    },
    onError: (e) =>
      setMsg({ type: 'error', text: e instanceof ApiError ? e.message : t('ldap.failedToSave') }),
  })

  const testM = useMutation({
    mutationFn: api.testLdap,
    onSuccess: (r) =>
      setMsg(
        r.ok
          ? { type: 'success', text: t('ldap.testOk') }
          : { type: 'error', text: t('ldap.testFailed', { error: r.error ?? '' }) },
      ),
    onError: (e) =>
      setMsg({ type: 'error', text: t('ldap.testFailed', { error: e instanceof ApiError ? e.message : '' }) }),
  })

  const importM = useMutation({
    mutationFn: api.importLdap,
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      setMsg({
        type: 'success',
        text: t('ldap.importDone', { created: r.created, existing: r.existing, scanned: r.scanned }),
      })
    },
    onError: (e) =>
      setMsg({ type: 'error', text: t('ldap.importFailed', { error: e instanceof ApiError ? e.message : '' }) }),
  })

  const busy = saveM.isPending || testM.isPending || importM.isPending
  const savedEnabled = data?.enabled ?? false

  return (
    <div className="card mt-8 p-5 sm:p-6">
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-white">{t('ldap.title')}</h2>
        <p className="mt-0.5 text-sm text-slate-500">{t('ldap.subtitle')}</p>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner className="h-6 w-6 text-accent-400" />
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            setMsg(null)
            saveM.mutate()
          }}
          className="space-y-4"
        >
          <Check
            label={t('ldap.enabled')}
            checked={form.enabled}
            onChange={(v) => setForm({ ...form, enabled: v })}
          />

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Text
              label={t('ldap.url')}
              help={t('ldap.urlHelp')}
              value={form.url}
              onChange={(v) => setForm({ ...form, url: v })}
              placeholder="ldaps://ldap.example.com:636"
            />
            <div className="flex items-end pb-2">
              <Check
                label={t('ldap.startTLS')}
                checked={form.startTLS}
                onChange={(v) => setForm({ ...form, startTLS: v })}
              />
            </div>

            <Text
              label={t('ldap.bindDN')}
              help={t('ldap.bindDNHelp')}
              value={form.bindDN}
              onChange={(v) => setForm({ ...form, bindDN: v })}
              placeholder="cn=reader,dc=example,dc=com"
            />
            <Text
              label={t('ldap.bindPassword')}
              help={passwordSet ? t('ldap.bindPasswordStored') : undefined}
              type="password"
              value={password}
              onChange={setPassword}
              placeholder={passwordSet ? t('ldap.bindPasswordKeep') : ''}
              autoComplete="new-password"
            />

            <Text
              label={t('ldap.baseDN')}
              help={t('ldap.baseDNHelp')}
              value={form.baseDN}
              onChange={(v) => setForm({ ...form, baseDN: v })}
              placeholder="ou=people,dc=example,dc=com"
            />
            <Text
              label={t('ldap.adminGroupDN')}
              help={t('ldap.adminGroupDNHelp')}
              value={form.adminGroupDN}
              onChange={(v) => setForm({ ...form, adminGroupDN: v })}
              placeholder="cn=admins,ou=groups,dc=example,dc=com"
            />

            <Text
              label={t('ldap.userFilter')}
              help={t('ldap.userFilterHelp')}
              value={form.userFilter}
              onChange={(v) => setForm({ ...form, userFilter: v })}
              placeholder="(uid=%s)"
            />
            <Text
              label={t('ldap.usernameAttribute')}
              help={t('ldap.usernameAttributeHelp')}
              value={form.usernameAttribute}
              onChange={(v) => setForm({ ...form, usernameAttribute: v })}
              placeholder="uid"
            />
          </div>

          {msg && (
            <div
              className={`rounded-xl border px-3.5 py-2.5 text-sm ${
                msg.type === 'success'
                  ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'
                  : 'border-red-500/30 bg-red-500/10 text-red-300'
              }`}
            >
              {msg.text}
            </div>
          )}

          <div className="flex flex-wrap items-center gap-2 pt-1">
            <button type="submit" className="btn-primary" disabled={busy}>
              {saveM.isPending && <Spinner className="h-4 w-4" />}
              {t('ldap.save')}
            </button>
            <button
              type="button"
              className="btn-secondary"
              disabled={busy || !savedEnabled}
              title={!savedEnabled ? t('ldap.saveFirst') : undefined}
              onClick={() => {
                setMsg(null)
                testM.mutate()
              }}
            >
              {testM.isPending && <Spinner className="h-4 w-4" />}
              {t('ldap.test')}
            </button>
            <button
              type="button"
              className="btn-secondary"
              disabled={busy || !savedEnabled}
              title={!savedEnabled ? t('ldap.saveFirst') : undefined}
              onClick={() => {
                setMsg(null)
                importM.mutate()
              }}
            >
              {importM.isPending && <Spinner className="h-4 w-4" />}
              {t('ldap.import')}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}

function Text({
  label,
  help,
  value,
  onChange,
  type = 'text',
  placeholder,
  autoComplete,
}: {
  label: string
  help?: string
  value: string
  onChange: (v: string) => void
  type?: string
  placeholder?: string
  autoComplete?: string
}) {
  return (
    <div>
      <label className="label">{label}</label>
      <input
        className="input"
        type={type}
        value={value}
        placeholder={placeholder}
        autoComplete={autoComplete}
        onChange={(e) => onChange(e.target.value)}
      />
      {help && <p className="mt-1 text-xs text-slate-500">{help}</p>}
    </div>
  )
}

function Check({
  label,
  checked,
  onChange,
}: {
  label: ReactNode
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
