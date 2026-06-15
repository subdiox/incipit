export function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / Math.pow(1024, i)
  return `${value.toFixed(value >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function formatDate(value?: string): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  // Calibre uses 0101-01-01 as an "unset" sentinel for dates.
  if (d.getFullYear() <= 1) return ''
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export function authorNames(authors: { name: string }[]): string {
  return authors.map((a) => a.name).join(', ') || 'Unknown author'
}

const LANGUAGE_NAMES: Intl.DisplayNames | null = (() => {
  try {
    return new Intl.DisplayNames(undefined, { type: 'language' })
  } catch {
    return null
  }
})()

export function languageLabel(code: string): string {
  if (!code) return ''
  try {
    return LANGUAGE_NAMES?.of(code) ?? code
  } catch {
    return code
  }
}
