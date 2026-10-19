export const STATUS_LABEL: Record<string, string> = {
  ok: 'OK',
  review: 'Needs review',
  no_speech: 'No speech',
  error: 'Error',
}

/** "1 clip", "2 clips". */
export const plural = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`

/** Last path segment of a "/" or "\" separated path. */
export function baseName(p: string): string {
  const s = p.replace(/[\\/]+$/, '')
  return s.slice(Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\')) + 1) || s
}

/** Parent directory of a "/" or "\" separated path ("/a/b/c" -> "/a/b", "/a" -> "/"). */
export function parentDir(p: string): string {
  const s = p.replace(/[\\/]+$/, '')
  const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'))
  return i < 0 ? '' : i === 0 ? s[0] : s.slice(0, i)
}

/** Directory part of a "/" separated relative path ("sub/x.mp3" -> "sub/"), "" if none. */
export function dirPart(rel: string): string {
  const i = rel.lastIndexOf('/')
  return i < 0 ? '' : rel.slice(0, i + 1)
}

const pad = (n: number) => String(n).padStart(2, '0')

/** 312 -> "05:12", 3725 -> "1:02:05"; "—" when unknown. */
export function clock(sec: number | null | undefined): string {
  if (sec == null || !isFinite(sec)) return '—'
  const t = Math.round(sec)
  const h = Math.floor(t / 3600)
  const m = Math.floor((t % 3600) / 60)
  return h ? `${h}:${pad(m)}:${pad(t % 60)}` : `${pad(m)}:${pad(t % 60)}`
}

/** 12480 -> "3h 28m"; seconds only below a minute. */
export function hm(sec: number): string {
  const t = Math.round(sec)
  if (t < 60) return `${t}s`
  const h = Math.floor(t / 3600)
  const m = Math.round((t % 3600) / 60)
  return h ? `${h}h ${m}m` : `${m}m`
}

/** Remaining-time estimate: "< 1 min", "~12 min", "~1 h 5 min". */
export function eta(sec: number): string {
  const m = Math.round(sec / 60)
  if (sec < 60) return '< 1 min'
  if (m < 60) return `~${m} min`
  return `~${Math.floor(m / 60)} h ${m % 60} min`
}

/** Probability/confidence with two decimals; "<0.01" for tiny non-zero values. */
export function prob(v: number | null | undefined): string {
  if (v == null) return '—'
  if (v > 0 && v < 0.005) return '<0.01'
  return v.toFixed(2)
}

/** "3 files (.txt, .jpg)"; "—" if nothing was skipped. The "" key is files without an extension. */
export function skippedText(skipped: Record<string, number> | null | undefined): string {
  const e = Object.entries(skipped ?? {}).sort((a, b) => b[1] - a[1])
  if (!e.length) return '—'
  const n = e.reduce((s, [, c]) => s + c, 0)
  const exts = e.slice(0, 3).map(([x]) => (x ? '.' + x : 'no ext.'))
  return `${plural(n, 'file')} (${exts.join(', ')}${e.length > 3 ? ', …' : ''})`
}

/** Error text from a rejected Wails call (a string) or an Error. */
export function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** Lower-case and strip diacritics so "viet" finds "Việt" (file names are often Vietnamese). */
export function fold(s: string): string {
  return s
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .replace(/đ/g, 'd')
    .replace(/Đ/g, 'D')
    .toLowerCase()
}
