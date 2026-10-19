package engine

import (
	"path/filepath"
	"runtime"
)

// AssetsIn returns a Config pointing into dir, a folder laid out the way
// scripts/fetch-assets.sh writes third_party/<goos>-<goarch>/. Thread settings stay zero.
func AssetsIn(dir string) Config {
	exe, lib := "", "libonnxruntime.so.1.29.0"
	switch runtime.GOOS {
	case "windows":
		exe, lib = ".exe", "onnxruntime.dll"
	case "darwin":
		lib = "libonnxruntime.1.29.0.dylib"
	}
	return Config{
		FFmpeg:  filepath.Join(dir, "ffmpeg", "bin", "ffmpeg"+exe),
		ORTLib:  filepath.Join(dir, "onnxruntime", lib),
		ECAPA:   filepath.Join(dir, "models", "voxlingua107.onnx"),
		LangMap: filepath.Join(dir, "models", "lang_map.json"),
		VAD:     filepath.Join(dir, "models", "silero_vad.onnx"),
	}
}
