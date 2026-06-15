import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { Spinner, FullPageSpinner } from '@/components/Spinner'
import { IconBook } from '@/components/icons'

export function LoginPage() {
  const navigate = useNavigate()
  const { user, loading: authLoading, setUser } = useAuth()

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ['setup-status'],
    queryFn: api.setupStatus,
    staleTime: Infinity,
  })

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const needsSetup = status?.needsSetup ?? false

  if (authLoading) return <FullPageSpinner />
  if (user) return <Navigate to="/" replace />

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)

    if (needsSetup) {
      if (password.length < 8) {
        setError('Password must be at least 8 characters.')
        return
      }
      if (password !== confirm) {
        setError('Passwords do not match.')
        return
      }
    }

    setSubmitting(true)
    try {
      const u = needsSetup ? await api.setup(username, password) : await api.login(username, password)
      setUser(u)
      navigate('/', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-gradient-to-b from-ink-950 to-ink-900 px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-accent-600 text-white shadow-glow">
            <IconBook width={28} height={28} />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight text-white">
            {needsSetup ? 'Welcome to Incipit' : 'Sign in to Incipit'}
          </h1>
          <p className="mt-1.5 text-sm text-slate-400">
            {needsSetup
              ? 'Create your administrator account to get started.'
              : 'Your self-hosted comic library.'}
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
                Username
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
                Password
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
                <p className="mt-1.5 text-xs text-slate-500">At least 8 characters.</p>
              )}
            </div>
            {needsSetup && (
              <div>
                <label className="label" htmlFor="confirm">
                  Confirm password
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

            {error && (
              <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-sm text-red-300">
                {error}
              </div>
            )}

            <button type="submit" className="btn-primary w-full" disabled={submitting}>
              {submitting && <Spinner className="h-4 w-4" />}
              {needsSetup ? 'Create account' : 'Sign in'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
