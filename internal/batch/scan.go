// Package batch turns folders into results: scan for clips, run them through
// a lid.Detector with a bounded worker pool, and report progress.
package batch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// Folder is one scanned root, a folder or a single file the user added: the
// clips to process and the files left out.
type Folder struct {
	Path    string         // exactly as the user gave it
	Name    string         // last path element, for display
	Dir     string         // folder the Files are relative to: Path, or a file's parent
	Files   []string       // matching clips, relative to Dir, "/"-separated, in walk order
	Skipped map[string]int // other visible files by lower-case extension ("" = none)
	Err     string         // set when Path itself cannot be read; Files is empty then
}

// ScanFolder lists the clips under root whose extension is in s.Extensions
// (case-insensitive), descending into sub-folders only if s.Recursive.
// Hidden files and directories (and so macOS "._" companions) are ignored and
// not counted as skipped. A root that is a file is one clip (or one skipped file).
func ScanFolder(root string, s lid.Settings) Folder {
	f := Folder{Path: root, Name: filepath.Base(root), Dir: root, Skipped: map[string]int{}}
	want := make(map[string]bool, len(s.Extensions))
	for _, e := range s.Extensions {
		want[strings.ToLower(strings.TrimPrefix(e, "."))] = true
	}
	st, err := os.Stat(root)
	switch {
	case err != nil:
		f.Err = err.Error()
		return f
	case !st.IsDir():
		f.Dir = filepath.Dir(root)
		if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(root), ".")); want[ext] {
			f.Files = []string{filepath.Base(root)}
		} else {
			f.Skipped[ext] = 1
		}
		return f
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if p == root {
			return err // the root was stat'ed above; a read error here is fatal for it
		}
		if err != nil {
			if f.Err == "" {
				f.Err = "some sub-folders could not be read: " + err.Error()
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if isHidden(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !s.Recursive {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(d.Name()), "."))
		if !want[ext] {
			f.Skipped[ext]++
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		f.Files = append(f.Files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		f.Err = err.Error()
		f.Files = nil
	}
	return f
}

// Summary is the screen-1 row for f. TotalDurationS is nil unless f has no
// clips (then it is known: 0); durations of real clips arrive later (Scanner).
func (f Folder) Summary() lid.FolderSummary {
	fs := lid.FolderSummary{Path: f.Path, Name: f.Name, Clips: len(f.Files), Skipped: f.Skipped, Error: f.Err}
	if f.Err == "" && len(f.Files) == 0 {
		zero := 0.0
		fs.TotalDurationS = &zero
	}
	return fs
}

// abs joins a Files entry back onto the folder it is relative to.
func (f Folder) abs(rel string) string { return filepath.Join(f.Dir, filepath.FromSlash(rel)) }

var errNoClips = errors.New("no clips to process")
