package main

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound API used by the frontend. Method signatures are the
// contract with the UI (generated into frontend/wailsjs/go/main/App.*).
//
// SCAFFOLD: bodies below are canned stubs so the UI can be built in parallel.
// The app peer replaces them with internal/batch; signatures change only via the lead.
type App struct {
	ctx context.Context
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// DefaultSettings returns the initial screen-1 settings.
func (a *App) DefaultSettings() lid.Settings {
	return lid.Settings{Extensions: lid.DefaultExtensions, MinConfidence: lid.DefaultMinConfidence}
}

// PickFolder opens the native folder picker; "" if the user cancels.
func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Chọn thư mục"})
}

// ScanFolders counts matching clips in each folder (screen 1 table).
// Dropped paths that are files, not folders, come back with Error set.
func (a *App) ScanFolders(paths []string, s lid.Settings) []lid.FolderSummary {
	out := make([]lid.FolderSummary, 0, len(paths))
	for _, p := range paths {
		d := 3600.0
		out = append(out, lid.FolderSummary{Path: p, Name: filepath.Base(p), Clips: 12, Skipped: map[string]int{"txt": 1}, TotalDurationS: &d})
	}
	return out
}

// Start runs detection over the folders; progress arrives as lid.EventProgress events.
func (a *App) Start(folders []string, s lid.Settings) error {
	if len(folders) == 0 {
		return errors.New("no folders")
	}
	go func() { // SCAFFOLD: fake progress
		for i := 1; i <= 12; i++ {
			time.Sleep(300 * time.Millisecond)
			runtime.EventsEmit(a.ctx, lid.EventProgress, a.stubProgress(i, 12))
		}
	}()
	return nil
}

func (a *App) Pause()  {}
func (a *App) Resume() {}
func (a *App) Cancel() {}

// Progress returns the current snapshot (same shape as the event payload).
func (a *App) Progress() lid.Progress { return a.stubProgress(0, 0) }

// Results returns every finished clip of the current/last run.
func (a *App) Results() []lid.ClipResult {
	c, sc := 0.93, 0.05
	return []lid.ClipResult{{Folder: "/tmp/demo", File: "a.mp3", Path: "/tmp/demo/a.mp3", Language: "vi", Confidence: &c, SecondLanguage: "en", SecondConfidence: &sc, Status: lid.StatusOK}}
}

// PickExportPath opens the native save dialog; "" if the user cancels.
func (a *App) PickExportPath(defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Lưu kết quả", DefaultFilename: defaultName})
}

// Export writes the selected results as JSON or CSV to req.Out.
func (a *App) Export(req lid.ExportRequest) error { return errors.New("not implemented") }

// OpenFileLocation reveals the file in Finder / Explorer.
func (a *App) OpenFileLocation(path string) error { return errors.New("not implemented") }

func (a *App) stubProgress(done, total int) lid.Progress {
	st := lid.StateRunning
	if done == total {
		st = lid.StateDone
	}
	return lid.Progress{State: st, Done: done, Total: total, Summary: lid.Summary{Total: done, OK: done}}
}
