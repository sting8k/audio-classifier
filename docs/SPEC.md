# Audio Language Classifier — spec

Desktop tool (Windows + macOS) that detects the spoken language of every audio/video clip
in a list of folders and exports the results as JSON or CSV.

Wireframe (4 screens: folders → processing → results → export): `docs/wireframe/`.
Where the wireframe and this spec disagree, **this spec wins** (see D-1).

## Decisions (settled with the user, 2026-10-19)

| ID | Decision | What future work must NOT do |
|---|---|---|
| D-1 | Analyse **at most the first 30 s** of each clip: decode → VAD → **one** model pass over the kept speech. | No per-segment timeline, no "segment length / min segment" settings, no "secondary language %" from segment voting, no "per-segment detail" export. |
| D-2 | Model: **Ear 0.2.1** (`desert-ant-labs/ear`, 103 languages), run as ONNX: our conversion of their `ear.tflite` (`scripts/ear-onnx`), same top-1 as the tflite on 450/450 FLEURS clips. Input: Whisper-style log-mel [1, 80, 3000] computed in Go from `mel_filters.f32`. Licence: Desert Ant Labs Source-Available 1.0 (settled with the user 2026-10-19). Replaced ECAPA VoxLingua107: on FLEURS (450 clips, 18 languages) 97.3% vs 93.6% correct, a third of the sure-but-wrong. | No FireRedLID, **no Whisper fallback**, no LiteRT runtime (one runtime: onnxruntime). |
| D-3 | Output only the base language code (`pt`, not pt-BR / pt-PT). | No dialect/region labels. |
| D-4 | GUI: **Wails** (Go backend, web frontend in the OS webview). Needs real paths: folder drag-drop, native folder picker, "open file location". | No local-webapp-in-browser, no Fyne/Gio. |
| D-5 | **CPU only.** | No GPU option / device selector, no GPU onnxruntime builds. |
| D-6 | Ship everything needed offline: model, Silero VAD model, onnxruntime shared lib, ffmpeg. Binaries/models are fetched at build time and published on GitHub Releases, not committed to git. | No first-run model download. |
| D-7 | "Needs review" when **top-1 minus top-2 probability < 0.25** (user-editable "Review margin"). Measured on FLEURS with Ear: 98% of clips pass, 7 of 450 pass while wrong. FLEURS is read speech, likely close to Ear's training data; not measured on ad audio. | Don't claim the rate holds on real ad clips. |
| D-8 | ffmpeg bundled, invoked with tuned args (see Pipeline). | — |
| D-9 | macOS is internal use only, **no Apple Developer account**: unsigned/ad-hoc signed app; users clear quarantine manually. | No notarization work. |
| D-10 | Frontend: **Svelte + TypeScript** (Wails v2 template, Wails v2.17.x — v3 is still beta). Plain CSS following the wireframe; IBM Plex fonts bundled locally (app is offline). | No UI component library, no state library, no router, no table virtualization (results are paginated); no web-font CDN; no Wails v3 until it is stable. |
| D-11 | All user-facing text is **English**: UI labels, status names, error messages written by the app. The wireframe's Vietnamese labels are translated, layout unchanged. | No Vietnamese UI strings, no i18n/language switcher. File and folder names are shown as they are. |

Second-best language: show the model's 2nd-highest class and its probability as "2nd choice"
(it is NOT a share of speech time).

## Pipeline (per clip)

1. **Decode** with bundled ffmpeg, audio only, first 30 s, straight to raw PCM on stdout:
   `ffmpeg -nostdin -hide_banner -loglevel info -nostats -t 30 -i file:<path> -map 0:a:0 -vn -sn -dn -ac 1 -ar 16000 -f f32le pipe:1`
   (`-loglevel info` so the same run prints `Duration:`, no ffprobe; `file:` so a name is never read as an option or protocol.)
   (`-t` before `-i` stops reading input after 30 s; `-map 0:a:0` picks the first audio track only.)
   No audio stream / decode failure → status `error`.
2. **VAD**: Silero VAD (ONNX, same onnxruntime). Keep speech regions only.
   No speech (or too little to classify) → status `no_speech`.
3. **Classify**: one Ear pass over the concatenated speech (zero-padded to 30 s) → softmax → top-1, top-2.
4. **Status**: `ok` if top-1 prob − top-2 prob ≥ review margin, else `review`.

Clips run in parallel through a bounded worker pool (one ffmpeg process per in-flight clip).

## Statuses

`ok` · `review` (Needs review) · `no_speech` (No speech) · `error` (Error: message).

## Inputs

- A list of folders (add via picker or drag-drop; remove; optional recursive scan).
- File extensions (checkboxes): mp3, wav, m4a, aac, flac, ogg, mp4, mov.

## Export

- JSON or CSV; scope: all / current results filter / selected folders; selectable fields.
- CSV option: "for Excel" = UTF-8 with BOM **and `;` as delimiter** (default on), because Excel on a Windows
  region with a decimal comma (e.g. Vietnamese) splits on `;`. Off = plain UTF-8, `,` delimiter.
  Unverified on a real vi-VN Excel; check one before release, including whether `0.9712` opens as a number
  (decimals are still written with `.`; if they open as text, use `,` decimals in Excel mode only).
- CSV only: a cell whose text starts with `=`, `+`, `-` or `@` gets a leading `'` so Excel does not run it as a formula
  (file names may come from outside). JSON keeps every value as is.
- Fields: folder, file, duration_s, language, confidence, second_language, second_confidence, status, error.
- JSON header: generated_at, model, settings (`max_seconds`, `min_margin`), summary counts.

## Platforms

Windows x64; macOS (arm64; x64 if needed). Builds per platform (cgo for onnxruntime) — macOS
binaries must be built on macOS (e.g. GitHub Actions runner).
