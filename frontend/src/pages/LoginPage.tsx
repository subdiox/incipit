import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate, type Location } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { useSiteTitle } from '@/lib/hooks'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { DirectoryPicker } from '@/components/DirectoryPicker'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconBook, IconFolder, IconEye, IconEyeOff } from '@/components/icons'

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useI18n()
  // Where to go after signing in: the page the user originally tried to open
  // (captured by RequireAuth), else the library home.
  const from = (location.state as { from?: Location } | null)?.from
  const redirectTo = from ? `${from.pathname}${from.search}${from.hash}` : '/'
  const siteTitle = useSiteTitle()
  const { user, loading: authLoading, setUser } = useAuth()

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ['setup-status'],
    queryFn: api.setupStatus,
    staleTime: Infinity,
  })

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [confirm, setConfirm] = useState('')
  const [libraryPath, setLibraryPath] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const needsSetup = status?.needsSetup ?? false
  const needsLibrary = status?.needsLibrary ?? false

  if (authLoading) return <FullPageSpinner />
  if (user) return <Navigate to={redirectTo} replace />

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
      navigate(redirectTo, { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('common.genericError'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-ink-950 px-4 py-12">
      {/* Ambient accent glow (behind the card, both themes). */}
      <div
        aria-hidden
        className="pointer-events-none absolute -top-40 left-1/2 h-[34rem] w-[34rem] -translate-x-1/2 rounded-full bg-accent-500/20 blur-[130px]"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -bottom-40 -right-24 h-96 w-96 rounded-full bg-accent-700/10 blur-[110px]"
      />

      <div className="relative w-full max-w-sm">
        <div className="mb-6 flex justify-end">
          <LanguageSwitcher />
        </div>
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-accent-400 to-accent-700 text-onaccent shadow-glow ring-1 ring-white/10">
            <IconBook width={30} height={30} />
          </span>
          <h1 className="text-[1.7rem] font-semibold tracking-tight text-white">
            {needsSetup
              ? t('login.welcome', { title: siteTitle })
              : t('login.signinTitle', { title: siteTitle })}
          </h1>
          {needsSetup && <p className="mt-2 text-sm text-slate-400">{t('login.setupSubtitle')}</p>}
        </div>

        {statusLoading ? (
          <div className="flex justify-center py-8">
            <Spinner className="h-6 w-6 text-accent-400" />
          </div>
        ) : (
          <form
            onSubmit={onSubmit}
            className="animate-fade-in space-y-4 rounded-2xl border border-ink-700 bg-ink-850/80 p-6 shadow-soft backdrop-blur-sm"
          >
            <div>
              <label className="label" htmlFor="username">
                {t('login.username')}
              </label>
              <input
                id="username"
                name="username"
                type="text"
                className="input"
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
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
              <div className="relative">
                <input
                  id="password"
                  name="password"
                  type={showPassword ? 'text' : 'password'}
                  className="input pr-10"
                  autoComplete={needsSetup ? 'new-password' : 'current-password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((v) => !v)}
                  className="absolute inset-y-0 right-0 flex items-center px-3 text-slate-500 transition-colors hover:text-slate-300"
                  aria-label={t(showPassword ? 'login.hidePassword' : 'login.showPassword')}
                  title={t(showPassword ? 'login.hidePassword' : 'login.showPassword')}
                  tabIndex={-1}
                >
                  {showPassword ? <IconEyeOff width={18} height={18} /> : <IconEye width={18} height={18} />}
                </button>
              </div>
              {needsSetup && <p className="mt-1.5 text-xs text-slate-500">{t('login.atLeast8')}</p>}
            </div>
            {needsSetup && (
              <div>
                <label className="label" htmlFor="confirm">
                  {t('login.confirmPassword')}
                </label>
                <input
                  id="confirm"
                  name="confirmPassword"
                  type={showPassword ? 'text' : 'password'}
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
                    name="library-path"
                    autoComplete="off"
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
