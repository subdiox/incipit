import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { useSiteTitle } from '@/lib/hooks'
import {
  IconAdmin,
  IconBook,
  IconFilter,
  IconHistory,
  IconLibrary,
  IconLogout,
  IconMenu,
  IconSearch,
  IconShelf,
  IconClose,
} from './icons'

function NavItem({
  to,
  icon,
  label,
  onClick,
}: {
  to: string
  icon: React.ReactNode
  label: string
  onClick?: () => void
}) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      onClick={onClick}
      className={({ isActive }) =>
        `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
          isActive
            ? 'bg-accent-500/15 text-accentSoft'
            : 'text-slate-400 hover:bg-ink-800 hover:text-white'
        }`
      }
    >
      {icon}
      <span>{label}</span>
    </NavLink>
  )
}

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { user, logout } = useAuth()
  const { t } = useI18n()
  const siteTitle = useSiteTitle()
  const navigate = useNavigate()
  const panes = useQuery({ queryKey: ['panes'], queryFn: api.panes }).data ?? []

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="flex h-full flex-col">
      <Link
        to="/"
        onClick={onNavigate}
        className="flex items-center gap-2.5 px-4 pb-6 pt-5"
      >
        <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-accent-600 text-onaccent shadow-glow">
          <IconBook width={20} height={20} />
        </span>
        <span className="truncate text-lg font-semibold tracking-tight text-white">{siteTitle}</span>
      </Link>

      <nav className="flex-1 space-y-1 px-2">
        <NavItem to="/" icon={<IconLibrary width={18} height={18} />} label={t('nav.library')} onClick={onNavigate} />
        {/* Admin-defined panes (saved filters) sit just under the library. */}
        {panes.map((p) => (
          <NavItem
            key={p.id}
            to={`/panes/${p.id}`}
            icon={<IconFilter width={16} height={16} />}
            label={p.name}
            onClick={onNavigate}
          />
        ))}
        <NavItem to="/shelves" icon={<IconShelf width={18} height={18} />} label={t('nav.shelves')} onClick={onNavigate} />
        <NavItem to="/history" icon={<IconHistory width={18} height={18} />} label={t('nav.history')} onClick={onNavigate} />
        {user?.isAdmin && (
          <NavItem to="/admin" icon={<IconAdmin width={18} height={18} />} label={t('nav.admin')} onClick={onNavigate} />
        )}
      </nav>

      <div className="border-t border-ink-700 p-2">
        <div className="flex items-center gap-1">
          <Link
            to="/account"
            onClick={onNavigate}
            title={t('nav.account')}
            className="flex min-w-0 flex-1 items-center gap-3 rounded-xl px-2 py-2 transition-colors hover:bg-ink-800"
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-ink-700 text-sm font-semibold uppercase text-accentSoft">
              {user?.username?.[0] ?? '?'}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-slate-200">{user?.username}</p>
              <p className="truncate text-[11px] text-slate-500">
                {user?.isAdmin ? t('nav.administrator') : t('nav.member')}
              </p>
            </div>
          </Link>
          <button
            type="button"
            onClick={handleLogout}
            className="rounded-lg p-1.5 text-slate-400 transition-colors hover:bg-ink-700 hover:text-white"
            aria-label={t('nav.logout')}
            title={t('nav.logout')}
          >
            <IconLogout width={18} height={18} />
          </button>
        </div>
      </div>
    </div>
  )
}

function TopBar({ onMenu }: { onMenu: () => void }) {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useI18n()
  const [value, setValue] = useState(params.get('search') ?? '')

  // Keep input in sync when navigating between pages.
  useEffect(() => {
    setValue(params.get('search') ?? '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.get('search')])

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const next = new URLSearchParams(params)
    if (value.trim()) next.set('search', value.trim())
    else next.delete('search')
    next.delete('offset')
    // Stay on the current pane page when searching there; otherwise the library.
    const pathname = location.pathname.startsWith('/panes/') ? location.pathname : '/'
    navigate({ pathname, search: `?${next.toString()}` })
  }

  return (
    <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-ink-800 bg-ink-950/80 px-4 backdrop-blur-md sm:px-6">
      <button
        type="button"
        onClick={onMenu}
        className="rounded-lg p-2 text-slate-300 hover:bg-ink-800 hover:text-white lg:hidden"
        aria-label={t('nav.openMenu')}
      >
        <IconMenu />
      </button>
      <form onSubmit={submit} className="relative w-full max-w-xl">
        <IconSearch
          className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500"
          width={18}
          height={18}
        />
        <input
          type="search"
          value={value}
          onChange={(e) => {
            const v = e.target.value
            setValue(v)
            // Live update only while on the library page via query param.
            const next = new URLSearchParams(params)
            if (v.trim()) next.set('search', v.trim())
            else next.delete('search')
            next.delete('offset')
            setParams(next, { replace: true })
          }}
          placeholder={t('nav.searchPlaceholder')}
          className="input pl-10"
        />
      </form>
    </header>
  )
}

export function Layout() {
  const { t } = useI18n()
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <div className="flex h-full">
      {/* Desktop sidebar */}
      <aside className="hidden w-64 shrink-0 border-r border-ink-800 bg-ink-900 lg:block">
        <Sidebar />
      </aside>

      {/* Mobile drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
            aria-hidden
          />
          <aside className="absolute inset-y-0 left-0 w-64 animate-fade-in border-r border-ink-800 bg-ink-900">
            <button
              type="button"
              onClick={() => setMobileOpen(false)}
              className="absolute right-3 top-3 rounded-lg p-1.5 text-slate-400 hover:bg-ink-700 hover:text-white"
              aria-label={t('nav.closeMenu')}
            >
              <IconClose width={18} height={18} />
            </button>
            <Sidebar onNavigate={() => setMobileOpen(false)} />
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar onMenu={() => setMobileOpen(true)} />
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6 lg:px-8">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
