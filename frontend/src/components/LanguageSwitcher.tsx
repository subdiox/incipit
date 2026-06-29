import { LANGUAGES, useI18n } from '@/i18n'

// Compact segmented control to switch the UI language. The choice is persisted
// per account (and locally) by the i18n provider.
export function LanguageSwitcher({ className = '' }: { className?: string }) {
  const { lang, setLang, t } = useI18n()
  return (
    <div
      className={`inline-flex rounded-lg border border-ink-700 bg-ink-800 p-0.5 ${className}`}
      role="group"
      aria-label={t('nav.language')}
    >
      {LANGUAGES.map((l) => (
        <button
          key={l.value}
          type="button"
          onClick={() => setLang(l.value)}
          aria-pressed={lang === l.value}
          className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
            lang === l.value
              ? 'bg-accent-600 text-onaccent'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          {l.label}
        </button>
      ))}
    </div>
  )
}
