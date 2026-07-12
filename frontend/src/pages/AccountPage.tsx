import { Link } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { TranslationKey } from '@/i18n/en'
import { useTheme, THEME_OPTIONS } from '@/lib/theme'
import { usePopularityEnabled, useReadingActivityEnabled } from '@/lib/hooks'
import type { SortKey, SortOrder } from '@/types'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { IconChevronLeft } from '@/components/icons'

const PAGE_SIZE_OPTIONS = [25, 50, 100]

// The default sort fields offered in settings (mirrors the library's menu).
const SORT_OPTIONS: { value: SortKey; labelKey: TranslationKey }[] = [
  { value: 'timestamp', labelKey: 'library.sort.recentlyAdded' },
  { value: 'pubdate', labelKey: 'library.sort.pubdate' },
  { value: 'rating', labelKey: 'library.sort.rating' },
  { value: 'favorites', labelKey: 'library.sort.favorites' },
  { value: 'views', labelKey: 'library.sort.views' },
  { value: 'lastread', labelKey: 'library.sort.lastread' },
]

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 px-5 py-4">
      <p className="text-sm font-medium text-slate-300">{label}</p>
      <p className="text-sm text-slate-200">{value}</p>
    </div>
  )
}

function ThemeSwitcher() {
  const { t } = useI18n()
  const { mode, setMode } = useTheme()
  return (
    <div
      className="inline-flex rounded-lg border border-ink-700 bg-ink-800 p-0.5"
      role="group"
      aria-label={t('account.theme')}
    >
      {THEME_OPTIONS.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => setMode(o.value)}
          aria-pressed={mode === o.value}
          className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
            mode === o.value ? 'bg-accent-600 text-onaccent' : 'text-slate-400 hover:text-fg'
          }`}
        >
          {t(o.labelKey)}
        </button>
      ))}
    </div>
  )
}

// Series-grouped browse toggle (was an inline library control; a per-account
// preference, so it lives here). Series = one tile per series; Volumes = every
// book individually.
function GroupingSwitcher() {
  const { t } = useI18n()
  const { user, setUser } = useAuth()
  const grouped = user?.groupSeries ?? true
  const set = (v: boolean) => {
    if (!user || v === (user.groupSeries ?? true)) return
    setUser({ ...user, groupSeries: v }) // optimistic
    api.setGroupSeries(v).then(setUser).catch(() => setUser(user))
  }
  return (
    <div
      className="inline-flex rounded-lg border border-ink-700 bg-ink-800 p-0.5"
      role="group"
      aria-label={t('account.grouping')}
    >
      {[
        { v: true, label: t('library.viewSeries') },
        { v: false, label: t('library.viewVolumes') },
      ].map((o) => (
        <button
          key={String(o.v)}
          type="button"
          onClick={() => set(o.v)}
          aria-pressed={grouped === o.v}
          className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
            grouped === o.v ? 'bg-accent-600 text-onaccent' : 'text-slate-400 hover:text-fg'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

// Per-page count (was an inline library control; a per-account preference).
function PageSizeSelect() {
  const { t } = useI18n()
  const { user, setUser } = useAuth()
  const size = user?.pageSize ?? 36
  const change = (n: number) => {
    if (!user || n === user.pageSize) return
    setUser({ ...user, pageSize: n }) // optimistic
    api.setPageSize(n).then(setUser).catch(() => setUser(user))
  }
  // Include the current value so a non-listed default (e.g. 36) still shows.
  const options = Array.from(new Set([...PAGE_SIZE_OPTIONS, size])).sort((a, b) => a - b)
  return (
    <select
      value={size}
      onChange={(e) => change(Number(e.target.value))}
      className="input w-auto cursor-pointer py-2 pr-8"
      aria-label={t('account.pageSize')}
    >
      {options.map((n) => (
        <option key={n} value={n}>
          {t('library.perPageOption', { count: n })}
        </option>
      ))}
    </select>
  )
}

// Default sort field + direction. This is the DEFAULT a library/collection page
// opens with; each page can still override it locally (via its own sort control,
// which only affects that view).
function DefaultSortControl() {
  const { t } = useI18n()
  const { user, setUser } = useAuth()
  const popularityOn = usePopularityEnabled()
  const readingActivityOn = useReadingActivityEnabled()
  const available = (v: SortKey) =>
    v === 'favorites' ? popularityOn : v === 'views' || v === 'lastread' ? readingActivityOn : true
  const sort = (user?.sort as SortKey) ?? 'timestamp'
  const order = (user?.sortOrder as SortOrder) ?? 'desc'
  const setSort = (v: SortKey) => {
    if (!user || v === user.sort) return
    setUser({ ...user, sort: v }) // optimistic
    api.setSort(v).then(setUser).catch(() => setUser(user))
  }
  const toggleOrder = () => {
    if (!user) return
    const next: SortOrder = order === 'desc' ? 'asc' : 'desc'
    setUser({ ...user, sortOrder: next }) // optimistic
    api.setSortOrder(next).then(setUser).catch(() => setUser(user))
  }
  return (
    <div className="flex items-center gap-2">
      <select
        value={sort}
        onChange={(e) => setSort(e.target.value as SortKey)}
        className="input w-auto cursor-pointer py-2 pr-8"
        aria-label={t('account.defaultSort')}
      >
        {SORT_OPTIONS.filter((o) => available(o.value)).map((o) => (
          <option key={o.value} value={o.value}>
            {t(o.labelKey)}
          </option>
        ))}
      </select>
      <button
        type="button"
        onClick={toggleOrder}
        className="btn-secondary shrink-0"
        title={order === 'desc' ? t('library.descending') : t('library.ascending')}
      >
        {order === 'desc' ? '↓' : '↑'}
      </button>
    </div>
  )
}

export function AccountPage() {
  const { user } = useAuth()
  const { t } = useI18n()

  return (
    <div className="mx-auto max-w-2xl">
      <Link to="/" className="btn-ghost mb-4 -ml-2 inline-flex">
        <IconChevronLeft width={18} height={18} />
        {t('nav.library')}
      </Link>

      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-white">{t('account.title')}</h1>
        <p className="mt-0.5 text-sm text-slate-500">{t('account.subtitle')}</p>
      </div>

      <div className="card divide-y divide-ink-800">
        <InfoRow label={t('account.username')} value={user?.username ?? ''} />
        <InfoRow
          label={t('account.role')}
          value={user?.isAdmin ? t('nav.administrator') : t('nav.member')}
        />
        <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium text-slate-300">{t('account.theme')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.themeHelp')}</p>
          </div>
          <ThemeSwitcher />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium text-slate-300">{t('account.language')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.languageHelp')}</p>
          </div>
          <LanguageSwitcher />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium text-slate-300">{t('account.grouping')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.groupingHelp')}</p>
          </div>
          <GroupingSwitcher />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium text-slate-300">{t('account.defaultSort')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.defaultSortHelp')}</p>
          </div>
          <DefaultSortControl />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium text-slate-300">{t('account.pageSize')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.pageSizeHelp')}</p>
          </div>
          <PageSizeSelect />
        </div>
      </div>
    </div>
  )
}
