// Pure helpers over the results list: filtering (screen 3), export scope + preview (screen 4).
import type { lid } from '../../wailsjs/go/models'
import { fold } from './format'

export type Clip = lid.ClipResult

export interface Filter {
  q: string
  folder: string // ClipResult.folder, "" = all
  lang: string // language code, "" = all
  status: string // "" = all
}

export const emptyFilter = (): Filter => ({ q: '', folder: '', lang: '', status: '' })

export function applyFilter(list: Clip[], f: Filter): Clip[] {
  const q = fold(f.q.trim())
  return list.filter(
    (c) =>
      (!f.folder || c.folder === f.folder) &&
      (!f.lang || c.language === f.lang) &&
      (!f.status || c.status === f.status) &&
      (!q || fold(c.file).includes(q)),
  )
}

export function countBy(list: Clip[]) {
  const s = { total: list.length, ok: 0, review: 0, no_speech: 0, error: 0 }
  for (const c of list) if (c.status in s) s[c.status as 'ok']++
  return s
}

/** Language distribution over clips that have a label (ok + review), most frequent first. */
export function languageShare(list: Clip[]): { code: string; n: number }[] {
  const m = new Map<string, number>()
  for (const c of list) if (c.language && (c.status === 'ok' || c.status === 'review')) m.set(c.language, (m.get(c.language) ?? 0) + 1)
  return [...m].map(([code, n]) => ({ code, n })).sort((a, b) => b.n - a.n || a.code.localeCompare(b.code))
}

export type Scope = 'all' | 'filter' | 'folders'

/** The clips an export would contain. */
export function inScope(list: Clip[], scope: Scope, filter: Filter, folders: string[]): Clip[] {
  if (scope === 'filter') return applyFilter(list, filter)
  if (scope === 'folders') return list.filter((c) => folders.includes(c.folder))
  return list
}

// SPEC "Export" fields, in file order. Keys are the ClipResult JSON tags.
export const EXPORT_FIELDS: { key: keyof Clip; label: string }[] = [
  { key: 'folder', label: 'Folder' },
  { key: 'file', label: 'File' },
  { key: 'duration_s', label: 'Duration (s)' },
  { key: 'language', label: 'Language' },
  { key: 'confidence', label: 'Confidence' },
  { key: 'second_language', label: '2nd choice' },
  { key: 'second_confidence', label: '2nd choice probability' },
  { key: 'status', label: 'Status' },
  { key: 'error', label: 'Error message' },
]

// "For Excel" CSV uses ";" (SPEC "Export"); plain CSV uses ",". Text starting with = + - @ gets a leading ' so Excel
// does not run it as a formula.
const csvCell = (v: unknown, sep: string) => {
  let s = v == null ? '' : String(v)
  if (/^[=+\-@]/.test(s)) s = `'${s}`
  return s.includes(sep) || /["\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

/** First rows as the CSV file would show them (illustrative: the real file is written by the backend). */
export function csvPreview(rows: Clip[], fields: (keyof Clip)[], forExcel: boolean): string {
  const sep = forExcel ? ';' : ','
  return [fields.join(sep), ...rows.map((c) => fields.map((k) => csvCell(c[k], sep)).join(sep))].join('\n')
}

/** Header + first rows as the JSON file would show them (illustrative, see csvPreview). */
export function jsonPreview(rows: Clip[], fields: (keyof Clip)[], total: number, minConfidence: number, summary: ReturnType<typeof countBy>): string {
  const clips = rows.map((c) => {
    const o: Record<string, unknown> = {}
    for (const k of fields) if (k !== 'error' || c.error) o[k] = c[k] ?? null
    return o
  })
  const body = JSON.stringify({ generated_at: new Date().toISOString(), model: 'voxlingua107-ecapa', settings: { max_seconds: 30, min_confidence: minConfidence }, summary, clips }, null, 2)
  return total > rows.length ? body.replace(/\n {2}\]/, `,\n    …\n  ]`) : body
}
