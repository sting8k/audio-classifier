// DEV ONLY. Stands in for the Wails bindings when the UI runs in a plain browser (`npm run dev`), so screens can be
// rendered and screenshotted without a display. main.ts imports this only under import.meta.env.DEV && !window.go;
// `vite build` removes that branch, so none of this reaches the production bundle.
// Mirrors the Go contract in internal/lid/api.go: canned data, a fake run of the 187 clips from the wireframe.
import type { lid } from '../../wailsjs/go/models'

type Clip = lid.ClipResult
type Cb = (...a: any[]) => void

const ALL_EXT = ['mp3', 'wav', 'm4a', 'aac', 'flac', 'ogg', 'mp4', 'mov']
const TICK_MS = 45 // one clip per tick
const EMIT_MS = 150 // progress event throttle
const TIME_SCALE = 40 // fake clock runs this much faster than the real one so ETA looks like a real batch

const FOLDERS: Record<string, { clips: number; skipped: Record<string, number>; hours: number }> = {
  '/Volumes/Data/Ads/2026-T09': { clips: 42, skipped: { txt: 2, jpg: 1 }, hours: 3.47 },
  '/Volumes/Data/Ads/Doi_tac_A': { clips: 118, skipped: {}, hours: 9.83 },
  '/Volumes/Data/Ads/Doi_tac_B': { clips: 27, skipped: { psd: 1, '': 2 }, hours: 2.25 },
  '/Users/linh/Documents/Quảng cáo Tết Nguyên Đán 2026/Bản cuối đã duyệt (Công ty TNHH Thương mại Hoàng Long)': { clips: 14, skipped: { docx: 3, pdf: 2, xlsx: 1, png: 4 }, hours: 0.6 },
}
const PICKS = Object.keys(FOLDERS)

// ---- deterministic data ----
function rng(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}
const LONG_NAMES = [
  'Quảng cáo Tết Nguyên Đán 2026 - bản dài (đã duyệt lần 3) - Công ty TNHH Thương mại và Dịch vụ Hoàng Long.mp3',
  'Phỏng vấn khách hàng Nguyễn Thị Thuỳ Dương về dịch vụ chăm sóc sau bán hàng tại Đà Nẵng.m4a',
  'Bản tin sáng thứ Hai 19-10-2026 (phiên bản phát thanh khu vực Đồng bằng sông Cửu Long).wav',
  'sub/Chiến dịch Giáng sinh/Đoạn 15 giây - phiên bản tiếng Anh.mp3',
  'Lời chúc năm mới của Giám đốc điều hành gửi toàn thể cán bộ nhân viên.mp4',
]
const ERRORS = [
  'could not read the file',
  'no audio stream',
  'ffmpeg: Invalid data found when processing input',
  'ffmpeg exited with status 1: moov atom not found; the file may be truncated, or it was still being written when it was copied from the camera card',
]
const LANGS: [string, number][] = [['vi', 72], ['en', 17], ['zh', 5], ['th', 3], ['ko', 0.8], ['ja', 0.7], ['id', 0.6], ['fr', 0.5], ['pt', 0.4], ['ru', 0.4], ['ar', 0.3], ['de', 0.3]]

function pickLang(r: () => number, not = ''): string {
  const pool = LANGS.filter(([c]) => c !== not)
  let x = r() * pool.reduce((s, [, w]) => s + w, 0)
  for (const [c, w] of pool) if ((x -= w) < 0) return c
  return pool[0][0]
}

function makeClips(folder: string, count: number, seed: number): Clip[] {
  const r = rng(seed)
  const out: Clip[] = []
  for (let i = 1; i <= count; i++) {
    const k = r()
    const ext = ['mp3', 'wav', 'm4a', 'mp4'][Math.floor(r() * 4)]
    const file = k < 0.14 ? LONG_NAMES[Math.floor(r() * LONG_NAMES.length)] : k < 0.3 ? `promo_${String(i).padStart(2, '0')}.${ext}` : `spot_${String(i).padStart(3, '0')}.${ext}`
    const dur = Math.round((15 + r() * 300) * 10) / 10
    const c: Partial<Clip> = { folder, file, path: `${folder}/${file}`, duration_s: dur, language: '', second_language: '', status: 'ok' }
    const s = r()
    if (s < 0.017) {
      c.status = 'error'
      c.error = ERRORS[Math.floor(r() * ERRORS.length)]
      delete c.duration_s
    } else if (s < 0.045) {
      c.status = 'no_speech'
      c.speech_s = 0
    } else {
      const lang = pickLang(r)
      const weak = r() < 0.06
      const conf = weak ? 0.35 + r() * 0.24 : 0.62 + 0.37 * Math.pow(r(), 0.5)
      c.language = lang
      c.confidence = Math.round(conf * 100) / 100
      c.second_language = pickLang(r, lang)
      c.second_confidence = Math.round((1 - conf) * r() * 0.9 * 100) / 100
      c.speech_s = Math.round(Math.min(30, dur * (0.4 + r() * 0.5)) * 10) / 10
      c.status = weak ? 'review' : 'ok'
    }
    out.push(c as Clip)
  }
  return out
}

// ---- event bus ----
const listeners = new Map<string, Set<Cb>>()
const emit = (name: string, ...a: any[]) => listeners.get(name)?.forEach((cb) => cb(...a))
let dropCb: Cb | null = null

// ---- fake backend state ----
const state = {
  scanSeq: 0,
  settings: null as lid.Settings | null,
  run: 'idle' as string,
  clips: [] as Clip[], // the whole queue
  done: 0,
  startedAt: 0, // fake seconds consumed before the current running stretch
  resumedAt: 0,
  timer: 0 as number,
  lastEmit: 0,
  pickIdx: 0,
  folderOrder: [] as string[],
}

const base = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p

function summaryFor(path: string, s: lid.Settings): lid.FolderSummary {
  const known = FOLDERS[path]
  const frac = (s.extensions.length / ALL_EXT.length) * (s.recursive ? 1 : 0.85)
  if (/\.[a-z0-9]{2,4}$/i.test(base(path))) return { path, name: base(path), clips: 1, skipped: {} } as lid.FolderSummary
  if (/khong_co_quyen/i.test(path)) return { path, name: base(path), clips: 0, skipped: {}, error: `open ${path}: permission denied` } as lid.FolderSummary
  const clips = known ? Math.round(known.clips * frac) : 8 + (path.length % 20)
  return { path, name: base(path), clips, skipped: known?.skipped ?? { txt: 1 } } as lid.FolderSummary
}

function folderClips(path: string): Clip[] {
  const n = summaryFor(path, state.settings!).clips
  return makeClips(path, n, [...path].reduce((h, ch) => (h * 31 + ch.charCodeAt(0)) | 0, 7))
}

function fakeElapsed(): number {
  const live = state.run === 'running' ? (Date.now() - state.resumedAt) / 1000 : 0
  return (state.startedAt + live) * TIME_SCALE
}

function snapshot(): lid.Progress {
  const idle = state.run === 'idle'
  const done = state.clips.slice(0, state.done)
  const sum = { total: done.length, ok: 0, review: 0, no_speech: 0, error: 0 }
  for (const c of done) sum[c.status as 'ok']++
  const el = fakeElapsed()
  const folders = state.folderOrder.map((p) => {
    const all = state.clips.filter((c) => c.folder === p)
    return { path: p, name: base(p), done: done.filter((c) => c.folder === p).length, total: all.length }
  })
  const cur = state.run === 'running' || state.run === 'paused' ? state.clips[state.done] : undefined
  return {
    state: state.run,
    done: state.done,
    total: state.clips.length,
    elapsed_s: el,
    eta_s: state.done > 2 && state.run === 'running' ? (el / state.done) * (state.clips.length - state.done) : undefined,
    current: cur?.path ?? '',
    // The Go stub sends nil slices before the first clip finishes (JSON null): keep that shape here.
    folders: idle || !folders.length ? null : folders,
    recent: done.length ? done.slice(-5).reverse() : null,
    summary: sum,
  } as unknown as lid.Progress
}

function tick() {
  if (state.run !== 'running') return
  state.done++
  const finished = state.done >= state.clips.length
  if (finished) {
    state.startedAt += (Date.now() - state.resumedAt) / 1000
    state.run = 'done'
    clearInterval(state.timer)
  }
  if (finished || Date.now() - state.lastEmit >= EMIT_MS) {
    state.lastEmit = Date.now()
    emit('progress', snapshot())
  }
}

const App = {
  DefaultSettings: async (): Promise<lid.Settings> => ({ extensions: [...ALL_EXT], recursive: false, min_margin: 0.25, workers: 0 }) as lid.Settings,

  PickFolder: async () => PICKS[state.pickIdx++ % PICKS.length],
  PickFiles: async () => ['/Users/demo/Downloads/clip 1.mp3', '/Users/demo/Downloads/clip 2.mp4'],

  ScanFolders: async (paths: string[], s: lid.Settings) => {
    const seq = ++state.scanSeq
    state.settings = s
    const out = paths.map((p) => summaryFor(p, s))
    // Durations are "probed in the background": null now, one folder_totals event per folder later.
    out.forEach((f, i) => {
      if (f.error) return
      const hours = FOLDERS[f.path]?.hours ?? 0.3
      const total = f.clips === 0 ? 0 : Math.round(hours * 3600 * (f.clips / (FOLDERS[f.path]?.clips ?? f.clips)))
      if (f.clips === 0) {
        f.total_duration_s = 0
        return
      }
      setTimeout(() => seq === state.scanSeq && emit('folder_totals', { ...f, total_duration_s: total }), 900 + i * 700)
    })
    return out
  },

  Start: async (folders: string[], s: lid.Settings) => {
    if (!folders.length) throw 'no folders'
    state.settings = s
    state.folderOrder = folders
    state.clips = folders.flatMap(folderClips)
    state.done = 0
    state.startedAt = 0
    state.resumedAt = Date.now()
    state.run = 'running'
    state.lastEmit = 0
    clearInterval(state.timer)
    state.timer = setInterval(tick, TICK_MS) as unknown as number
  },
  Pause: async () => {
    if (state.run !== 'running') return
    state.startedAt += (Date.now() - state.resumedAt) / 1000
    state.run = 'paused'
    emit('progress', snapshot())
  },
  Resume: async () => {
    if (state.run !== 'paused') return
    state.resumedAt = Date.now()
    state.run = 'running'
  },
  Cancel: async () => {
    if (state.run !== 'running' && state.run !== 'paused') return
    if (state.run === 'running') state.startedAt += (Date.now() - state.resumedAt) / 1000
    state.run = 'cancelled'
    clearInterval(state.timer)
    emit('progress', snapshot())
  },
  Progress: async () => snapshot(),
  Results: async () => (state.clips.slice(0, state.done).length ? state.clips.slice(0, state.done) : null),

  PickExportPath: async (name: string) => `/Volumes/Data/Ads/${name}`,
  Export: async (req: lid.ExportRequest) => {
    mock.lastExport = req
  },
  OpenFileLocation: async (path: string) => {
    mock.lastOpen = path
  },
}

const runtime = {
  EventsOnMultiple: (name: string, cb: Cb) => {
    if (!listeners.has(name)) listeners.set(name, new Set())
    listeners.get(name)!.add(cb)
    return () => listeners.get(name)?.delete(cb)
  },
  EventsOff: (name: string) => listeners.delete(name),
  EventsEmit: emit,
  OnFileDrop: (cb: Cb) => (dropCb = cb),
  OnFileDropOff: () => (dropCb = null),
  CanResolveFilePaths: () => false,
  ResolveFilePaths: () => {},
}

/** Handles for scripted checks from the browser console / agent-browser eval. */
const mock = {
  lastExport: null as lid.ExportRequest | null,
  lastOpen: '',
  /** Simulate dropping folders on the window (what Wails' OnFileDrop would deliver). */
  drop: (paths: string[]) => dropCb?.(0, 0, paths),
  state,
}

Object.assign(window, { go: { main: { App } }, runtime, __LID_DEV_MOCK__: mock })
console.info('LID dev mock active (no Wails bindings found)')
