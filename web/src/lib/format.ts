export function ago(iso: string): string {
  const t = Date.parse(iso)
  if (!t || t < 0) return '–'
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return `${Math.floor(s)}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 172800) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export function when(iso: string): string {
  const d = new Date(iso)
  if (isNaN(+d)) return '–'
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// left is the time until iso, for an end shown while it runs: "12m", "1h30m", or "closing".
export function left(iso: string): string {
  const s = (Date.parse(iso) - Date.now()) / 1000
  if (!(s >= 60)) return 'closing'
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m`
  return m % 60 ? `${Math.floor(m / 60)}h${m % 60}m` : `${m / 60}h`
}

/** "a", "a and b", "a, b and c". */
export function names(list: string[]): string {
  return list.length < 2 ? (list[0] ?? '') : `${list.slice(0, -1).join(', ')} and ${list.at(-1)}`
}
