import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { useSiteTitle } from '@/lib/hooks'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { DirectoryPicker } from '@/components/DirectoryPicker'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconBook, IconFolder } from '@/components/icons'

export function LoginPage() {
  const navigate = useNavigate()
  const { t } = useI18n()
  const siteTitle = useSiteTitle()
  const { user, loading: authLoading, setUser } = useAuth()

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ['setup-status'],
    queryFn: api.setupStatus,
    staleTime: Infinity,
  })

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [libraryPath, setLibraryPath] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const needsSetup = status?.needsSetup ?? false
  const needsLibrary = status?.needsLibrary ?? false

  if (authLoading) return <FullPageSpinner />
  if (user) return <Navigate to="/" replace />

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)

    if (needsSetup) {
      if (password.length < 8) {
        setError(t('login.passwordTooShort'))
        return
      }
      if (password !== confirm) {
        setError(t('login.passwordMismatch'))
        return
      }
      if (needsLibrary && !libraryPath.trim()) {
        setError(t('login.libraryPathRequired'))
        return
      }
    }

    setSubmitting(true)
    try {
      const u = needsSetup
        ? await api.setup(username, password, needsLibrary ? libraryPath.trim() : undefined)
        : await api.login(username, password)
      setUser(u)
      navigate('/', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('common.genericError'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-gradient-to-b from-ink-950 to-ink-900 px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <LanguageSwitcher />
        </div>
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-accent-600 text-onaccent shadow-glow">
            <IconBook width={28} height={28} />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight text-white">
            {needsSetup
              ? t('login.welcome', { title: siteTitle })
              : t('login.signinTitle', { title: siteTitle })}
          </h1>
          <p className="mt-1.5 text-sm text-slate-400">
            {needsSetup ? t('login.setupSubtitle') : t('login.subtitle')}
          </p>
        </div>

        {statusLoading ? (
          <div className="flex justify-center py-8">
            <Spinner className="h-6 w-6 text-accent-400" />
          </div>
        ) : (
          <form onSubmit={onSubmit} className="card animate-fade-in space-y-4 p-6 shadow-soft">
            <div>
              <label className="label" htmlFor="username">
                {t('login.username')}
              </label>
              <input
                id="username"
                className="input"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <div>
              <label className="label" htmlFor="password">
                {t('login.password')}
              </label>
              <input
                id="password"
                type="password"
                className="input"
                autoComplete={needsSetup ? 'new-password' : 'current-password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
              {needsSetup && (
                <p className="mt-1.5 text-xs text-slate-500">{t('login.atLeast8')}</p>
              )}
            </div>
            {needsSetup && (
              <div>
                <label className="label" htmlFor="confirm">
                  {t('login.confirmPassword')}
                </label>
                <input
                  id="confirm"
                  type="password"
                  className="input"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  required
                />
              </div>
            )}

            {needsSetup && needsLibrary && (
              <div>
                <label className="label" htmlFor="libraryPath">
                  {t('login.libraryPath')}
                </label>
                <div className="flex gap-2">
                  <input
                    id="libraryPath"
                    className="input flex-1"
                    value={libraryPath}
                    onChange={(e) => setLibraryPath(e.target.value)}
                    placeholder="/library"
                    required
                  />
                  <button
                    type="button"
                    className="btn-secondary shrink-0"
                    onClick={() => setPickerOpen(true)}
                  >
                    <IconFolder width={16} height={16} />
                    {t('picker.browse')}
                  </button>
                </div>
                <p className="mt-1.5 text-xs text-slate-500">{t('login.libraryPathHelp')}</p>
                <DirectoryPicker
                  open={pickerOpen}
                  initialPath={libraryPath}
                  onClose={() => setPickerOpen(false)}
                  onSelect={(p) => setLibraryPath(p)}
                />
              </div>
            )}

            {error && (
              <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
                {error}
              </div>
            )}

            <button type="submit" className="btn-primary w-full" disabled={submitting}>
              {submitting && <Spinner className="h-4 w-4" />}
              {needsSetup ? t('login.createAccount') : t('login.signin')}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
