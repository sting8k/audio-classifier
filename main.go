package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sting8k/audio-classifier/internal/engine"
	"github.com/sting8k/audio-classifier/internal/lid"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	var app *App
	if os.Getenv("LANGID_FAKE") == "1" {
		// For UI work without the models; the delay makes pause/cancel visible.
		fake := lid.Fake{Delay: 300 * time.Millisecond}
		app = NewApp(fake, fake)
	} else {
		dir, err := assetsDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		cfg := engine.AssetsIn(dir)
		// One worker per core, one ORT thread each: fastest and cheapest in CPU in the
		// engine benchmark (6-core box); the shared session keeps RAM ~1 GB at 6 workers.
		cfg.IntraOpThreads = 1
		eng, err := engine.New(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: loading the engine:", err)
			os.Exit(1)
		}
		app = NewApp(eng, eng)
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "LangID Batch",
		Width:     1280,
		Height:    860,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// assetsDir finds the bundled ffmpeg/onnxruntime/models (layout of scripts/fetch-assets.sh):
// $LANGID_ASSETS, else third_party/<os>-<arch> next to the executable, two levels up
// (wails build puts it in build/bin) or in the working directory.
// TODO(packaging): settle where installers put the assets.
func assetsDir() (string, error) {
	if d := os.Getenv("LANGID_ASSETS"); d != "" {
		return d, nil
	}
	plat := filepath.Join("third_party", runtime.GOOS+"-"+runtime.GOARCH)
	var tried []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		tried = append(tried, filepath.Join(d, plat), filepath.Join(d, "..", "..", plat))
	}
	tried = append(tried, plat)
	for _, d := range tried {
		if _, err := os.Stat(filepath.Join(d, "models")); err == nil {
			return d, nil
		}
	}
	return "", fmt.Errorf("assets not found (run scripts/fetch-assets.sh or set LANGID_ASSETS); tried %v", tried)
}
