// Shared UI state (Svelte 5 runes in a module; no state library, SPEC D-10) and the actions that talk to the Go API.
import * as api from '../../wailsjs/go/main/App.js'
import { EventsOn, OnFileDrop } from '../../wailsjs/runtime/runtime.js'
import type { lid } from '../../wailsjs/go/models'
import { errText } from './format'
import { emptyFilter, EXPORT_FIELDS, type Clip, type Filter, type Scope } from './results'

// Wails event names (internal/lid/api.go: EventProgress, EventFolderTotals).
const EVENT_PROGRESS = 'progress'
const EVENT_FOLDER_TOTALS = 'folder_totals'

// Events and Go replies are plain JSON, not lid.Progress instances (that class carries a converter method).
export type ProgressData = Omit<lid.Progress, 'convertValues'>

export type Screen = 'folders' | 'running' | 'results' | 'export'
export const nav = $state({ screen: 'folders' as Screen })

/** Screen 1: folders and settings. */
export const input = $state({
  ready: false,
  offered: [] as string[], // extensions offered as checkboxes
  settings: { extensions: [], recursive: false, min_confidence: 0.6, workers: 0 } as lid.Settings,
  paths: [] as string[], // folders as added
  folders: [] as lid.FolderSummary[], // last ScanFolders answer, same order as paths
  totals: {} as Record<string, { clips: number; seconds: number }>, // folder_totals events by path; valid while clips match the row
  scanning: false,
  error: '',
})

/** Screen 2: the latest progress snapshot. */
export const run = $state({
  progress: null as ProgressData | null,
  minConfidence: 0.6, // threshold the run was started with
  busy: false, // a pause/resume/cancel call is in flight
  error: '',
})

/** Screen 3: results, filters, selection. */
export const res = $state({
  list: [] as Clip[],
  loading: false,
  error: '',
  filter: emptyFilter() as Filter,
  page: 0,
  selected: '', // ClipResult.path
})

/** Screen 4: export options. */
export const exp = $state({
  format: 'json' as 'json' | 'csv',
  scope: 'all' as Scope,
  folders: [] as string[], // ClipResult.folder values, for scope "folders"
  fields: EXPORT_FIELDS.map((f) => f.key as string),
  bom: true,
  out: '',
  busy: false,
  done: '', // path of the last successful export
  error: '',
})

export const hasRun = () => run.progress != null && run.progress.state !== 'idle'
export const isActive = () => run.progress?.state === 'running' || run.progress?.state === 'paused'

// ---------- screen 1 ----------

let scanSeq = 0
let scanTimer: ReturnType<typeof setTimeout> | undefined

/** Ask the backend to count clips. A newer call supersedes older ones, so the latest answer wins. */
export async function rescan(resetTotals = false) {
  clearTimeout(scanTimer)
  const seq = ++scanSeq
  if (resetTotals) input.totals = {}
  if (!input.paths.length) {
    input.folders = []
    input.scanning = false
    return
  }
  input.scanning = true
  try {
    const out = await api.ScanFolders($state.snapshot(input.paths), $state.snapshot(input.settings))
    if (seq !== scanSeq) return
    input.folders = out ?? []
    input.error = ''
  } catch (e) {
    if (seq === scanSeq) input.error = errText(e)
  } finally {
    if (seq === scanSeq) input.scanning = false
  }
}

/** Extensions/recursive changed: rescan after a short pause so rapid clicks make one call. */
export function rescanSoon() {
  clearTimeout(scanTimer)
  scanTimer = setTimeout(() => rescan(true), 250)
}

export function addFolders(paths: string[]) {
  const fresh = [...new Set(paths)].filter((p) => p && !input.paths.includes(p))
  if (!fresh.length) return
  input.paths.push(...fresh)
  rescan()
}

export async function pickFolder() {
  try {
    const p = await api.PickFolder()
    if (p) addFolders([p])
  } catch (e) {
    input.error = errText(e)
  }
}

export function removeFolder(path: string) {
  input.paths = input.paths.filter((p) => p !== path)
  input.folders = input.folders.filter((f) => f.path !== path)
  rescan()
}

export function clearFolders() {
  input.paths = []
  rescan()
}

/** Known total duration of a folder: from the scan, or from a folder_totals event computed for the same clip count. */
export function folderSeconds(f: lid.FolderSummary): number | undefined {
  if (f.total_duration_s != null) return f.total_duration_s
  const t = input.totals[f.path]
  return t && t.clips === f.clips ? t.seconds : undefined
}

/** Folders that will run: those with clips. A folder can carry an error and still have clips (some sub-folders unreadable). */
export const runnable = () => input.folders.filter((f) => f.clips > 0)

/** Clip count and (known) duration over the folders that will run. */
export function plan() {
  let clips = 0
  let seconds = 0
  let unknown = 0
  for (const f of runnable()) {
    clips += f.clips
    const t = folderSeconds(f)
    if (t == null) unknown++
    else seconds += t
  }
  return { clips, seconds, unknown }
}

export async function start(): Promise<string> {
  const folders = runnable().map((f) => f.path)
  const before = run.progress
  run.error = ''
  run.minConfidence = input.settings.min_confidence
  run.progress = { state: 'running', done: 0, total: plan().clips, elapsed_s: 0, current: '', folders: [], recent: [], summary: { total: 0, ok: 0, review: 0, no_speech: 0, error: 0 } }
  try {
    await api.Start(folders, $state.snapshot(input.settings))
  } catch (e) {
    run.progress = before
    return errText(e)
  }
  res.list = []
  res.filter = emptyFilter()
  res.page = 0
  res.selected = ''
  exp.scope = 'all'
  exp.folders = []
  exp.done = ''
  exp.error = ''
  nav.screen = 'running'
  return ''
}

// ---------- screen 2 ----------

export async function syncProgress() {
  try {
    run.progress = await api.Progress()
  } catch (e) {
    run.error = errText(e)
  }
}

export async function control(call: () => Promise<void>) {
  run.busy = true
  run.error = ''
  try {
    await call()
  } catch (e) {
    run.error = errText(e)
  }
  await syncProgress()
  run.busy = false
}
export const pause = () => control(api.Pause)
export const resume = () => control(api.Resume)
export const cancel = () => control(api.Cancel)

// ---------- navigation, results ----------

export async function loadResults() {
  res.loading = true
  try {
    res.list = (await api.Results()) ?? []
    res.error = ''
  } catch (e) {
    res.error = errText(e)
  } finally {
    res.loading = false
  }
}

export async function go(s: Screen) {
  nav.screen = s
  window.scrollTo(0, 0)
  if (s === 'results' || s === 'export') await loadResults()
}

// ---------- boot ----------

export async function init() {
  EventsOn(EVENT_PROGRESS, (p: ProgressData) => {
    const was = run.progress?.state
    run.progress = p
    const finished = p.state === 'done' || p.state === 'cancelled'
    if (finished && was !== p.state && (nav.screen === 'results' || nav.screen === 'export')) loadResults()
  })
  EventsOn(EVENT_FOLDER_TOTALS, (f: lid.FolderSummary) => {
    if (f.total_duration_s != null) input.totals[f.path] = { clips: f.clips, seconds: f.total_duration_s }
  })
  // Drops count only on elements carrying `--wails-drop-target: drop` (screen 1 sets it).
  OnFileDrop((_x, _y, paths) => addFolders(paths), true)

  const [defaults, progress] = await Promise.all([api.DefaultSettings(), api.Progress()])
  input.offered = defaults.extensions ?? []
  input.settings = { ...defaults, extensions: [...(defaults.extensions ?? [])] }
  input.ready = true
  if (progress && progress.state !== 'idle') {
    run.progress = progress
    nav.screen = 'running'
  }
}
