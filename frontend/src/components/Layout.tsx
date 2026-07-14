import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { useRankingsEnabled, useSiteTitle } from '@/lib/hooks'
import {
  IconAdmin,
  IconBook,
  IconFilter,
  IconFlame,
  IconHistory,
  IconLibrary,
  IconLogout,
  IconMenu,
  IconSearch,
  IconShelf,
  IconClose,
} from './icons'

// The header hosts a portal target for the library's Filters control (so it sits
// next to the search bar). Because the header UNMOUNTS while hidden on scroll,
// that DOM node comes and goes — so its element is published through context and
// re-read by the library, which re-targets its portal whenever the header remounts.
const FilterSlotContext = createContext<HTMLElement | null>(null)
// eslint-disable-next-line react-refresh/only-export-components
export function useFilterSlot() {
  return useContext(FilterSlotContext)
}

function NavItem({
  to,
  icon,
  label,
  onClick,
  indent,
}: {
  to: string
  icon: React.ReactNode
  label: string
  onClick?: () => void
  indent?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      onClick={onClick}
      className={({ isActive }) =>
        `flex items-center gap-3 rounded-xl py-2.5 pr-3 text-sm font-medium transition-colors ${
          // Collections are children of Library, so indent them a little to the right.
          indent ? 'pl-7' : 'pl-3'
        } ${
          isActive
            ? 'bg-accent-500/15 text-accentSoft'
            : 'text-slate-400 hover:bg-ink-800 hover:text-white'
        }`
      }
    >
      {icon}
      <span className="truncate">{label}</span>
    </NavLink>
  )
}

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { user, logout } = useAuth()
  const { t } = useI18n()
  const siteTitle = useSiteTitle()
  const navigate = useNavigate()
  const collections = useQuery({ queryKey: ['collections'], queryFn: api.collections }).data ?? []
  // Rankings: a self-contained section (its own /rankings page with period tabs),
  // shown only when the admin enabled the feature AND the library actually has
  // ranking lists — so a comic instance never sees it.
  const rankingsOn = useRankingsEnabled()
  const rankings = useQuery({ queryKey: ['rankings'], queryFn: api.rankings, enabled: rankingsOn }).data ?? []
  const showRankings = rankingsOn && rankings.length > 0

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

      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2">
        <NavItem to="/" icon={<IconLibrary width={18} height={18} />} label={t('nav.library')} onClick={onNavigate} />
        {/* Admin-defined collections (saved filters) sit just under the library.
            The URL uses the 1-based display position so it tracks reordering. */}
        {collections.map((c, i) => (
          <NavItem
            key={c.id}
            to={`/collections/${i + 1}`}
            icon={<IconFilter width={16} height={16} />}
            label={c.name}
            onClick={onNavigate}
            indent
          />
        ))}
        {/* Rankings: its own top-level section, separate from the indented
            collections above. Periods are tabs within the page. */}
        {showRankings && (
          <NavItem to="/rankings" icon={<IconFlame width={18} height={18} />} label={t('nav.rankings')} onClick={onNavigate} />
        )}
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

// useHideOnScroll hides the header when scrolling down and reveals it on the
// first scroll up (or near the top), driven by the native window scroll.
function useHideOnScroll() {
  const [hidden, setHidden] = useState(false)
  const last = useRef(0)
  useEffect(() => {
    const onScroll = () => {
      // Desktop (lg+) has a persistent sidebar, so keep the header pinned there.
      if (window.matchMedia('(min-width: 1024px)').matches) {
        setHidden(false)
        return
      }
      const y = window.scrollY
      if (y < 8) setHidden(false)
      else if (y > last.current + 6) setHidden(true)
      else if (y < last.current - 6) setHidden(false)
      last.current = y
    }
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])
  return hidden
}

function TopBar({
  onMenu,
  slotRef,
}: {
  onMenu: () => void
  slotRef: (el: HTMLElement | null) => void
}) {
  const [params, setParams] = useSearchParams()
  const location = useLocation()
  const { t } = useI18n()
  const [value, setValue] = useState(params.get('search') ?? '')

  // Keep input in sync when navigating between pages.
  useEffect(() => {
    setValue(params.get('search') ?? '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.get('search')])

  // The search box filters whatever the current page shows: the library and
  // collections (library search), shelves (across all shelves) and history (by book
  // title). Other pages (settings, account, book detail) have nothing to search.
  const path = location.pathname
  const searchable = path === '/' || path.startsWith('/collections/') || path === '/shelves' || path === '/history'
  const placeholder =
    path === '/shelves'
      ? t('nav.searchShelves')
      : path === '/history'
        ? t('nav.searchHistory')
        : t('nav.searchPlaceholder')

  const setSearch = (v: string) => {
    setValue(v)
    const next = new URLSearchParams(params)
    if (v.trim()) next.set('search', v.trim())
    else next.delete('search')
    next.delete('offset')
    setParams(next, { replace: true })
  }

  return (
    // On iOS, any element pinned to the top edge (position:fixed OR sticky, even
    // translated off-screen) stops page content from scrolling up under the
    // status bar. So this header is fully UNMOUNTED while hidden (see Layout) —
    // it only exists when shown — and slides in via animate-slide-down instead of
    // a translate transition. The glass (bg + blur + border) sits on an absolute
    // child so Safari 26's "Liquid Glass" chrome doesn't sample a fixed element's
    // own background and tint the status bar with an opaque band.
    <header
      className={`fixed inset-x-0 top-0 z-30 animate-slide-down lg:left-64 ${
        searchable ? '' : 'lg:hidden'
      }`}
    >
      <div className="pointer-events-none absolute inset-0 border-b border-ink-800 bg-ink-950/80 backdrop-blur-md" />
      <div
        className="relative flex items-center gap-3 px-4 py-3 sm:px-6"
        style={{ paddingTop: 'calc(0.75rem + env(safe-area-inset-top))' }}
      >
        <button
          type="button"
          onClick={onMenu}
          className="rounded-lg p-2 text-slate-300 hover:bg-ink-800 hover:text-white lg:hidden"
          aria-label={t('nav.openMenu')}
        >
          <IconMenu />
        </button>
        {searchable && (
          <form onSubmit={(e) => e.preventDefault()} className="relative max-w-xl flex-1">
            <IconSearch
              className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500"
              width={18}
              height={18}
            />
            <input
              type="search"
              value={value}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={placeholder}
              className="input pl-10"
            />
          </form>
        )}
        {/* The library mounts its Filters control here, to the right of search. */}
        <div id="library-filter-slot" ref={slotRef} className="shrink-0" />
      </div>
    </header>
  )
}

export function Layout() {
  const { t } = useI18n()
  const [mobileOpen, setMobileOpen] = useState(false)
  const headerHidden = useHideOnScroll()
  const location = useLocation()
  // Current DOM node of the header's Filters-portal target. A callback ref keeps
  // it in sync as the header mounts/unmounts, so the library re-targets its
  // portal instead of writing into a detached node.
  const [filterSlot, setFilterSlot] = useState<HTMLElement | null>(null)

  // The header is fixed and fully unmounted while hidden (so iOS lets content
  // bleed under the status bar). Measure its height while it exists to pad the
  // content beneath it (varies with the search box and the safe-area inset), and
  // KEEP the last value when it unmounts so the content offset stays stable.
  const [headerH, setHeaderH] = useState(0)
  useEffect(() => {
    const el = document.querySelector('header')
    // When the header is unmounted (hidden), keep the last measured height so the
    // content's top offset stays stable and nothing jumps.
    if (!el) return
    const measure = () => setHeaderH(el.offsetHeight)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [location.pathname, headerHidden])

  return (
    <FilterSlotContext.Provider value={filterSlot}>
    <div className="min-h-full">
      {/* Desktop sidebar: fixed so the page scrolls natively behind it. */}
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-64 border-r border-ink-800 bg-ink-900 lg:block">
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

      <div className="min-w-0 lg:pl-64">
        {!headerHidden && <TopBar onMenu={() => setMobileOpen(true)} slotRef={setFilterSlot} />}
        <main className="overflow-x-clip">
          <div
            className="mx-auto w-full max-w-[1600px] px-4 sm:px-6 lg:px-8"
            style={{
              paddingTop: `calc(${headerH}px + 1.5rem)`,
              paddingBottom: 'calc(1.5rem + env(safe-area-inset-bottom))',
            }}
          >
            <Outlet />
          </div>
        </main>
      </div>
    </div>
    </FilterSlotContext.Provider>
  )
}
