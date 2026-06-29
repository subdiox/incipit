import { Link } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { useI18n } from '@/i18n'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { IconChevronLeft } from '@/components/icons'

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 px-5 py-4">
      <p className="text-sm font-medium text-slate-300">{label}</p>
      <p className="text-sm text-slate-200">{value}</p>
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
            <p className="text-sm font-medium text-slate-300">{t('account.language')}</p>
            <p className="mt-0.5 text-xs text-slate-500">{t('account.languageHelp')}</p>
          </div>
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  )
}
