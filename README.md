# Audio Language Classifier

A desktop app that tells you which language is spoken in each of your audio and video files.
Add folders or files, press Start, and export the results to CSV or JSON.
It runs offline on your own computer, using only the CPU.

## Download

Get the zip for your system from [Releases](https://github.com/sting8k/audio-classifier/releases/latest):

| System | File |
|---|---|
| macOS (Apple Silicon: M1, M2…) | `…-darwin-arm64.zip` |
| macOS (Intel) | `…-darwin-amd64.zip` |
| Windows (64-bit) | `…-windows-amd64.zip` |
| Linux (x86-64) | `…-linux-amd64.tar.gz` |

Unzip and run. No installation is needed.

- **macOS**: the app is not signed. Run `xattr -cr "Audio Language Classifier.app"` once in Terminal, then open it.
- **Windows**: keep the `third_party` folder next to `AudioLangClassifier.exe`.
- **Linux**: needs `libwebkit2gtk-4.1`.

## How it works

1. **Input**: drag in folders or files (mp3, wav, m4a, aac, flac, ogg, mp4, mov).
2. **Detection**: the app reads the first 30 seconds of each clip, keeps only the parts with speech, and
   picks the most likely language out of 107. You can pause or cancel at any time.
3. **Results**: each clip gets a language code (`en`, `vi`, `pt`…), a confidence score and a second choice.
   - **Needs review**: the confidence is below the threshold (0.60 by default, which is a guess, not a measured value).
   - **No speech**: the clip has less than about 3 seconds of speech.
4. **Export**: CSV or JSON. "CSV for Excel" uses `;` as the separator, so it opens correctly in Excel set to
   a region that writes decimals with a comma.

Only the base language is reported (`pt`, not pt-BR or pt-PT). The results come from a model, so check the
clips marked "Needs review".

## Build from source

Needs Go 1.25, Node 22 and [Wails](https://wails.io) v2.17.

```sh
scripts/fetch-assets.sh        # downloads pinned ffmpeg, onnxruntime and models into third_party/
wails build                    # add -tags webkit2_41 on Linux
```

Design and decisions: [docs/SPEC.md](docs/SPEC.md).

## License

MIT, see [LICENSE](LICENSE). The bundled third-party components keep their own licences, see
`third_party/<platform>/licenses`: onnxruntime (MIT), Silero VAD (MIT),
SpeechBrain ECAPA VoxLingua107 (Apache-2.0), and ffmpeg (LGPL v3 on Windows/Linux, GPL v3 on macOS).
