package main

import (
	"embed"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// TODO(integration): swap in internal/engine; LANGID_FAKE=1 keeps lid.Fake.
	// The delay makes pause/cancel visible while the UI is built against the fake.
	eng := lid.Fake{Delay: 300 * time.Millisecond}
	app := NewApp(eng, eng)

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
