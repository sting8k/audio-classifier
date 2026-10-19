package batch

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sting8k/audio-classifier/internal/lid"
)

func TestScanFolderRules(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "a.mp3", "B.WAV", "notes.txt", "x.TXT", "README",
		".hidden.mp3", "._a.mp3", ".git/e.mp3", // hidden, AppleDouble: ignored and not counted
		"sub/c.mp3", "sub/d.jpg", "tiếng Việt/f.m4a")
	set := lid.Settings{Extensions: []string{"mp3", "wav", "m4a"}}

	flat := ScanFolder(root, set)
	if want := []string{"B.WAV", "a.mp3"}; !reflect.DeepEqual(flat.Files, want) {
		t.Errorf("flat files = %v, want %v (case-insensitive ext, no sub-folders)", flat.Files, want)
	}
	if want := map[string]int{"txt": 2, "": 1}; !reflect.DeepEqual(flat.Skipped, want) {
		t.Errorf("flat skipped = %v, want %v", flat.Skipped, want)
	}

	set.Recursive = true
	deep := ScanFolder(root, set)
	if want := []string{"B.WAV", "a.mp3", "sub/c.mp3", "tiếng Việt/f.m4a"}; !reflect.DeepEqual(deep.Files, want) {
		t.Errorf("recursive files = %v, want %v", deep.Files, want)
	}
	if want := map[string]int{"txt": 2, "": 1, "jpg": 1}; !reflect.DeepEqual(deep.Skipped, want) {
		t.Errorf("recursive skipped = %v, want %v", deep.Skipped, want)
	}
}

// A vanished folder must surface as a row error, not as an empty folder that looks fine.
func TestScanFolderUnreadableRoot(t *testing.T) {
	f := ScanFolder(filepath.Join(t.TempDir(), "gone"), mp3)
	if f.Err == "" || len(f.Files) != 0 {
		t.Errorf("Err=%q Files=%v, want an error and no files", f.Err, f.Files)
	}
	if f.Skipped == nil {
		t.Errorf("Skipped must be non-nil (the UI iterates it)")
	}
}

// A file added on its own is one clip that resolves back to that file, or one
// skipped file when its extension is not selected.
func TestScanSingleFile(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "Tiếng Việt.MP3")
	touch(t, root, "notes.txt")
	p := filepath.Join(root, "Tiếng Việt.MP3")
	f := ScanFolder(p, mp3)
	if f.Err != "" || len(f.Files) != 1 {
		t.Fatalf("Err=%q Files=%v, want one clip", f.Err, f.Files)
	}
	if got := f.abs(f.Files[0]); got != p {
		t.Errorf("clip resolves to %q, want %q", got, p)
	}
	txt := ScanFolder(filepath.Join(root, "notes.txt"), mp3)
	if txt.Err != "" || len(txt.Files) != 0 || txt.Skipped["txt"] != 1 {
		t.Errorf("non-matching file: Err=%q Files=%v Skipped=%v, want 1 skipped txt", txt.Err, txt.Files, txt.Skipped)
	}
}
