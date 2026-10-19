#!/usr/bin/env bash
# Fetch the runtime assets the app bundles (SPEC D-6) into third_party/<goos>-<goarch>/.
# Every download is pinned by URL and sha256; a mismatch aborts before the old target dir is touched.
#
#   scripts/fetch-assets.sh                 # host target only
#   scripts/fetch-assets.sh all             # linux-amd64 windows-amd64 darwin-arm64
#   scripts/fetch-assets.sh darwin-arm64 windows-amd64
#
# Needs: bash, curl, tar, and unzip | bsdtar | python3 (for .zip), sha256sum | shasum.
# Layout written (must match engine.AssetsIn):
#   ffmpeg/bin/ffmpeg[.exe] ffprobe[.exe]   (+ ffmpeg/lib/*.so.N on linux, *.dll beside the exe on windows)
#   onnxruntime/<shared lib>
#   models/voxlingua107.onnx lang_map.json silero_vad.onnx
#   licenses/                               (texts that ship in the archives + model notices)
#
# What is bundled and under which licence:
#   onnxruntime 1.29.0 (CPU)  MIT. Must equal the C API header version of the Go binding
#                             github.com/yalue/onnxruntime_go v1.36.0 (ORT_API_VERSION 29).
#                             No darwin-amd64: Microsoft publishes no osx-x86_64 build for 1.29.0.
#   ffmpeg 9.0.2              linux/windows: BtbN "lgpl-shared" build, LGPL v3 (configure has
#                             --enable-version3, no --enable-gpl/--enable-nonfree), loaded as shared
#                             libs. darwin-arm64: martin-riedl.de static build, **GPL v3** (no LGPL
#                             prebuilt for macOS arm64 exists; D-9 makes macOS internal-only).
#                             ffmpeg runs as a separate process, our code is not linked to it.
#   Silero VAD v6.2.3         MIT.   ECAPA VoxLingua107 ONNX: Apache-2.0.
#
# BtbN deletes old autobuild tags after a while: when the URL below 404s, mirror the pinned
# archive to our GitHub Release (sha256 stays the same) or bump the pin.
set -euo pipefail

ORT_VER=1.29.0
ORT_URL=https://github.com/microsoft/onnxruntime/releases/download/v$ORT_VER
ORT_LINUX_AMD64_SHA=c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba
ORT_WINDOWS_AMD64_SHA=c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2
ORT_DARWIN_ARM64_SHA=d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92

BTBN_TAG=autobuild-2026-10-18-13-01
BTBN_REL=ffmpeg-n9.0.2-28-g669f5ab083
BTBN_URL=https://github.com/BtbN/FFmpeg-Builds/releases/download/$BTBN_TAG
FFMPEG_LINUX_AMD64_SHA=993c81efe48ac0b4a5a31fee8d2ed9c81687eee9d27fbc9f98c675f4a3cb1e37
FFMPEG_WINDOWS_AMD64_SHA=bc5a0e82b643605ae067f10aacdb4fcd27747779deefa52275d600695b2b1d48
MR_URL=https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2
FFMPEG_DARWIN_ARM64_SHA=c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924
FFPROBE_DARWIN_ARM64_SHA=fcbe839537485eaee7a7a8bc5cbc0f90d53617e80943e8a5b2e31cb851197ea6

HF_REV=e02e1da805ae49635fe1aa7913c3f1e7f5f5fde6   # christopherthompson81/voxlingua107-lid-onnx
HF_URL=https://huggingface.co/christopherthompson81/voxlingua107-lid-onnx/resolve/$HF_REV
ECAPA_SHA=e2c3c3da39b99e3f9196d15fceef6a65f702320038bbc08813a4f21280255ce8
LANGMAP_SHA=d593825e7e8ce82c8b50c0810a371a3e69cd0293b52751ddf515f6ee2c592182
ECAPA_README_SHA=71b97e47d91ffc47c0010c95b7dfa5d52bbae23214aa6bb9c45d175d0bf0d0dc
SILERO_REV=5cd7945676eb32225748052e2e6a0580e4686a08   # snakers4/silero-vad tag v6.2.3
SILERO_URL=https://raw.githubusercontent.com/snakers4/silero-vad/$SILERO_REV
SILERO_SHA=1a153a22f4509e292a94e67d6f9b85e8deb25b4988682b7e174c65279d8788e3
SILERO_LICENSE_SHA=2e63e9a38b6e8fc0c7bc37ce174caca1862870856c6daf5697cfb785e925520b

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CACHE=$ROOT/third_party/.cache
mkdir -p "$CACHE"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# fetch URL SHA256 -> sets FETCHED to the verified cached file. Re-downloads when the cache is
# missing or wrong. Not run in a $(...) subshell: an `exit 1` there would not stop the script.
fetch() {
  local url=$1 want=$2 out="$CACHE/$(echo "$1" | sed 's/[^A-Za-z0-9._-]/_/g' | tail -c 120)"
  if [ ! -f "$out" ] || [ "$(sha256_of "$out")" != "$want" ]; then
    echo "  download $url" >&2
    curl -fsSL --retry 3 --connect-timeout 20 -o "$out.part" "$url"
    mv "$out.part" "$out"
  fi
  local got; got=$(sha256_of "$out")
  if [ "$got" != "$want" ]; then
    rm -f "$out"
    echo "sha256 mismatch for $url: want $want got $got" >&2
    exit 1
  fi
  FETCHED=$out
}

unzip_to() { # archive dest
  if command -v unzip >/dev/null 2>&1; then unzip -q -o "$1" -d "$2"
  elif command -v bsdtar >/dev/null 2>&1; then bsdtar -xf "$1" -C "$2"
  else python3 -c 'import sys,zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])' "$1" "$2"; fi
}

fetch_to() { # url sha dest_file
  fetch "$1" "$2"; cp "$FETCHED" "$3"
}

fetch_models() { # dest
  mkdir -p "$1/models" "$1/licenses"
  fetch_to "$HF_URL/voxlingua107.onnx" $ECAPA_SHA "$1/models/voxlingua107.onnx"
  fetch_to "$HF_URL/lang_map.json" $LANGMAP_SHA "$1/models/lang_map.json"
  fetch_to "$SILERO_URL/src/silero_vad/data/silero_vad.onnx" $SILERO_SHA "$1/models/silero_vad.onnx"
  fetch_to "$HF_URL/README.md" $ECAPA_README_SHA "$1/licenses/voxlingua107-README.md"   # carries license: apache-2.0
  fetch_to "$SILERO_URL/LICENSE" $SILERO_LICENSE_SHA "$1/licenses/silero-vad-LICENSE"
}

ort_licenses() { # unpacked_dir dest
  cp "$1/LICENSE" "$2/licenses/onnxruntime-LICENSE"
  cp "$1/ThirdPartyNotices.txt" "$2/licenses/onnxruntime-ThirdPartyNotices.txt"
}

fetch_ort() { # target dest
  local t=$1 dest=$2 tmp; tmp=$(mktemp -d); mkdir -p "$dest/onnxruntime" "$dest/licenses"
  case $t in
    linux-amd64)
      fetch "$ORT_URL/onnxruntime-linux-x64-$ORT_VER.tgz" $ORT_LINUX_AMD64_SHA; tar -xzf "$FETCHED" -C "$tmp"
      cp "$tmp/onnxruntime-linux-x64-$ORT_VER/lib/libonnxruntime.so.$ORT_VER" "$dest/onnxruntime/"
      ort_licenses "$tmp/onnxruntime-linux-x64-$ORT_VER" "$dest" ;;
    windows-amd64)
      fetch "$ORT_URL/onnxruntime-win-x64-$ORT_VER.zip" $ORT_WINDOWS_AMD64_SHA; unzip_to "$FETCHED" "$tmp"
      cp "$tmp/onnxruntime-win-x64-$ORT_VER/lib/"{onnxruntime.dll,onnxruntime_providers_shared.dll} "$dest/onnxruntime/"
      ort_licenses "$tmp/onnxruntime-win-x64-$ORT_VER" "$dest" ;;
    darwin-arm64)
      fetch "$ORT_URL/onnxruntime-osx-arm64-$ORT_VER.tgz" $ORT_DARWIN_ARM64_SHA; tar -xzf "$FETCHED" -C "$tmp"
      cp "$tmp/onnxruntime-osx-arm64-$ORT_VER/lib/libonnxruntime.$ORT_VER.dylib" "$dest/onnxruntime/"
      ort_licenses "$tmp/onnxruntime-osx-arm64-$ORT_VER" "$dest" ;;
  esac
  rm -rf "$tmp"
}

fetch_ffmpeg() { # target dest
  local t=$1 dest=$2 tmp; tmp=$(mktemp -d); mkdir -p "$dest/ffmpeg/bin" "$dest/licenses"
  case $t in
    linux-amd64)
      fetch "$BTBN_URL/$BTBN_REL-linux64-lgpl-shared-9.0.tar.xz" $FFMPEG_LINUX_AMD64_SHA
      tar -xJf "$FETCHED" -C "$tmp" --strip-components=1
      mkdir -p "$dest/ffmpeg/lib"
      cp "$tmp/bin/ffmpeg" "$tmp/bin/ffprobe" "$dest/ffmpeg/bin/"
      # binaries have RPATH $ORIGIN/../lib; keep one real file per soname (cp -L), no symlinks.
      for so in "$tmp"/lib/lib*.so.[0-9]*; do
        case $(basename "$so") in *.*.*.*.*) continue ;; esac   # skip libX.so.N.M.P full names
        cp -L "$so" "$dest/ffmpeg/lib/"
      done
      cp "$tmp/LICENSE.txt" "$dest/licenses/ffmpeg-LICENSE.txt" ;;
    windows-amd64)
      fetch "$BTBN_URL/$BTBN_REL-win64-lgpl-shared-9.0.zip" $FFMPEG_WINDOWS_AMD64_SHA
      unzip_to "$FETCHED" "$tmp"
      local d; d=$(echo "$tmp"/ffmpeg-*)
      cp "$d/bin/ffmpeg.exe" "$d/bin/ffprobe.exe" "$d/bin/"*.dll "$dest/ffmpeg/bin/"
      cp "$d/LICENSE.txt" "$dest/licenses/ffmpeg-LICENSE.txt" ;;
    darwin-arm64)
      fetch "$MR_URL/ffmpeg.zip" $FFMPEG_DARWIN_ARM64_SHA; unzip_to "$FETCHED" "$tmp"
      fetch "$MR_URL/ffprobe.zip" $FFPROBE_DARWIN_ARM64_SHA; unzip_to "$FETCHED" "$tmp"
      cp "$tmp/ffmpeg" "$tmp/ffprobe" "$dest/ffmpeg/bin/"
      chmod +x "$dest/ffmpeg/bin/ffmpeg" "$dest/ffmpeg/bin/ffprobe"
      echo "ffmpeg for darwin-arm64 is GPL v3 (martin-riedl.de static build, FFmpeg 9.0.2); source and build script: https://ffmpeg.martin-riedl.de" > "$dest/licenses/ffmpeg-NOTICE.txt" ;;
  esac
  rm -rf "$tmp"
}

host_target() {
  local os arch
  case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; MINGW*|MSYS*|CYGWIN*) os=windows ;; *) echo "unknown OS" >&2; exit 1 ;; esac
  case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "unknown arch" >&2; exit 1 ;; esac
  echo "$os-$arch"
}

targets=("$@")
[ ${#targets[@]} -eq 0 ] && targets=("$(host_target)")
[ "${targets[0]}" = all ] && targets=(linux-amd64 windows-amd64 darwin-arm64)

for t in "${targets[@]}"; do
  case $t in linux-amd64|windows-amd64|darwin-arm64) ;; *) echo "unsupported target $t (darwin-amd64: no onnxruntime $ORT_VER build exists)" >&2; exit 1 ;; esac
  dest=$ROOT/third_party/$t
  echo "== $t -> $dest"
  rm -rf "$dest.tmp"; mkdir -p "$dest.tmp"   # build aside, swap in only when everything verified
  fetch_models "$dest.tmp"
  fetch_ort "$t" "$dest.tmp"
  fetch_ffmpeg "$t" "$dest.tmp"
  rm -rf "$dest"; mv "$dest.tmp" "$dest"
done
echo done
