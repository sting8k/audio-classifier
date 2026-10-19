package main

import (
	"context"
	"time"

	"github.com/sting8k/audio-classifier/internal/batch"
	"github.com/sting8k/audio-classifier/internal/export"
	"github.com/sting8k/audio-classifier/internal/lid"
	"github.com/sting8k/audio-classifier/internal/reveal"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound API used by the frontend. Method signatures are the
// contract with the UI (generated into frontend/wailsjs/go/main/App.*) and
// change only via the lead. The bodies are thin: the work is in internal/batch,
// internal/export and internal/reveal.
type App struct {
	ctx     context.Context
	det     lid.Detector
	runner  *batch.Runner
	scanner *batch.Scanner
}

// NewApp wires the app to an engine. The Detector and Prober are usually the
// same object (internal/engine, or lid.Fake).
func NewApp(det lid.Detector, prober lid.Prober) *App {
	a := &App{det: det, scanner: batch.NewScanner(prober)}
	a.runner = batch.NewRunner(batch.Config{
		Detector:   det,
		Prober:     prober,
		OnProgress: func(p lid.Progress) { runtime.EventsEmit(a.ctx, lid.EventProgress, p) },
	})
	return a
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown stops the run, waits for the workers to leave the engine, then closes it.
func (a *App) shutdown(context.Context) {
	a.scanner.Cancel()
	a.runner.Shutdown(5 * time.Second)
	a.det.Close()
}

// DefaultSettings returns the initial screen-1 settings.
func (a *App) DefaultSettings() lid.Settings {
	return lid.Settings{Extensions: lid.DefaultExtensions, MinMargin: lid.DefaultMinMargin}
}

// PickFolder opens the native folder picker; "" if the user cancels.
func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose a folder"})
}

// PickFiles opens the native multi-file picker filtered to the clip formats; nil if the user cancels.
func (a *App) PickFiles() ([]string, error) {
	pat := ""
	for i, e := range lid.DefaultExtensions {
		if i > 0 {
			pat += ";"
		}
		pat += "*." + e
	}
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose audio or video files",
		Filters: []runtime.FileFilter{{DisplayName: "Audio and video", Pattern: pat}},
	})
}

// ScanFolders counts matching clips in each folder or file (screen 1 table) and returns
// at once with total_duration_s null for folders that have clips. Durations are
// probed in the background and each folder's total arrives as a
// lid.EventFolderTotals event; call ScanFolders again (with every folder)
// whenever the list or the extension/recursive settings change.
// A path that is a file counts as one clip (or one skipped file).
func (a *App) ScanFolders(paths []string, s lid.Settings) []lid.FolderSummary {
	return a.scanner.Scan(paths, s, func(fs lid.FolderSummary) {
		runtime.EventsEmit(a.ctx, lid.EventFolderTotals, fs)
	})
}

// Start runs detection over the folders; progress arrives as lid.EventProgress events.
// It fails if a run is active or the folders hold no clips.
func (a *App) Start(folders []string, s lid.Settings) error {
	a.scanner.Cancel() // the run needs the CPU more than the totals do
	return a.runner.Start(folders, s)
}

// Pause stops handing out new clips; clips in flight finish.
func (a *App) Pause() { a.runner.Pause() }

func (a *App) Resume() { a.runner.Resume() }

// Cancel stops now and keeps the results finished so far.
func (a *App) Cancel() { a.runner.Cancel() }

// Progress returns the current snapshot (same shape as the event payload).
func (a *App) Progress() lid.Progress { return a.runner.Progress() }

// Results returns every finished clip of the current/last run, in scan order.
func (a *App) Results() []lid.ClipResult { return a.runner.Results() }

// PickExportPath opens the native save dialog; "" if the user cancels.
func (a *App) PickExportPath(defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Save results", DefaultFilename: defaultName})
}

// Export writes the selected results as JSON or CSV to req.Out.
func (a *App) Export(req lid.ExportRequest) error {
	return export.Write(req, a.runner.Results(), export.Options{MinMargin: a.runner.Settings().MinMargin})
}

// OpenFileLocation reveals the file in Finder / Explorer (on Linux, opens its folder).
func (a *App) OpenFileLocation(path string) error { return reveal.File(path) }
