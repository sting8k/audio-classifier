# LangID Batch — spec

Desktop tool (Windows + macOS) that detects the spoken language of every audio/video clip
in a list of folders and exports the results as JSON or CSV.

Wireframe (4 screens: folders → processing → results → export): `docs/wireframe/`.
Where the wireframe and this spec disagree, **this spec wins** (see D-1).

## Decisions (settled with the user, 2026-10-19)

| ID | Decision | What future work must NOT do |
|---|---|---|
| D-1 | Analyse **at most the first 30 s** of each clip: decode → VAD → **one** model pass over the kept speech. | No per-segment timeline, no "segment length / min segment" settings, no "secondary language %" from segment voting, no "per-segment detail" export. |
| D-2 | Model: SpeechBrain ECAPA VoxLingua107, ONNX export `christopherthompson81/voxlingua107-lid-onnx` (raw 16 kHz mono float32 in, 107-class logits out). | No FireRedLID, **no Whisper fallback**. |
| D-3 | Output only the base language code (`pt`, not pt-BR / pt-PT). | No dialect/region labels. |
| D-4 | GUI: **Wails** (Go backend, web frontend in the OS webview). Needs real paths: folder drag-drop, native folder picker, "open file location". | No local-webapp-in-browser, no Fyne/Gio. |
| D-5 | **CPU only.** | No GPU option / device selector, no GPU onnxruntime builds. |
| D-6 | Ship everything needed offline: model, Silero VAD model, onnxruntime shared lib, ffmpeg. Binaries/models are fetched at build time and published on GitHub Releases, not committed to git. | No first-run model download. |
| D-7 | Confidence threshold default **0.60** (a guess, not measured), user-editable. Below it → status "Cần xem lại" (review). | Don't present 0.60 as validated. |
| D-8 | ffmpeg bundled, invoked with tuned args (see Pipeline). | — |
| D-9 | macOS is internal use only, **no Apple Developer account**: unsigned/ad-hoc signed app; users clear quarantine manually. | No notarization work. |

Second-best language: show the model's 2nd-highest class and its probability as "Lựa chọn thứ 2"
(it is NOT a share of speech time).

## Pipeline (per clip)

1. **Decode** with bundled ffmpeg, audio only, first 30 s, straight to raw PCM on stdout:
   `ffmpeg -nostdin -hide_banner -loglevel error -t 30 -i <file> -map 0:a:0 -vn -sn -dn -ac 1 -ar 16000 -f f32le pipe:1`
   (`-t` before `-i` stops reading input after 30 s; `-map 0:a:0` picks the first audio track only.)
   No audio stream / decode failure → status `error`.
2. **VAD**: Silero VAD (ONNX, same onnxruntime). Keep speech regions only.
   No speech (or too little to classify) → status `no_speech`.
3. **Classify**: one ECAPA pass over the concatenated speech → softmax → top-1, top-2.
4. **Status**: `ok` if top-1 prob ≥ threshold, else `review`.

Clips run in parallel through a bounded worker pool (one ffmpeg process per in-flight clip).

## Statuses

`ok` · `review` (Cần xem lại) · `no_speech` (Không có giọng nói) · `error` (Lỗi: message).

## Inputs

- A list of folders (add via picker or drag-drop; remove; optional recursive scan).
- File extensions (checkboxes): mp3, wav, m4a, aac, flac, ogg, mp4, mov.

## Export

- JSON or CSV; scope: all / current results filter / selected folders; selectable fields.
- CSV option: UTF-8 with BOM (default on) for Excel on Windows.
- Fields: folder, file, duration_s, language, confidence, second_language, second_confidence, status, error.
- JSON header: generated_at, model, settings (`max_seconds`, `min_confidence`), summary counts.

## Platforms

Windows x64; macOS (arm64; x64 if needed). Builds per platform (cgo for onnxruntime) — macOS
binaries must be built on macOS (e.g. GitHub Actions runner).
