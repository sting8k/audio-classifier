package batch

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

func recv(t *testing.T, ch <-chan lid.FolderSummary) lid.FolderSummary {
	t.Helper()
	select {
	case fs := <-ch:
		return fs
	case <-time.After(5 * time.Second):
		t.Fatal("no folder_totals event")
		return lid.FolderSummary{}
	}
}

// ScanFolders must answer from the directory walk alone; durations follow later.
func TestScanReturnsBeforeDurationsAndReportsTotalsLater(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "a.mp3", "b.mp3", "bad.mp3")
	release := make(chan struct{})
	sc := NewScanner(probeFunc(func(_ context.Context, p string) (float64, error) {
		<-release
		if filepath.Base(p) == "bad.mp3" {
			return 0, errors.New("unreadable")
		}
		return 2.5, nil
	}))
	got := make(chan lid.FolderSummary, 4)
	sums := sc.Scan([]string{root, t.TempDir()}, mp3, func(fs lid.FolderSummary) { got <- fs })

	if len(sums) != 2 || sums[0].Clips != 3 || sums[0].TotalDurationS != nil {
		t.Fatalf("sums[0] = %+v, want 3 clips and no total yet", sums[0])
	}
	if sums[1].TotalDurationS == nil || *sums[1].TotalDurationS != 0 {
		t.Errorf("empty folder total = %v, want a known 0 (nothing to wait for)", sums[1].TotalDurationS)
	}
	close(release)
	fs := recv(t, got)
	if fs.Path != root || fs.Clips != 3 || fs.TotalDurationS == nil || *fs.TotalDurationS != 5 {
		t.Errorf("event = %+v, want %s total 5 (clips that failed to probe add nothing)", fs, root)
	}
}

// After a newer Scan has returned, the older one must never report.
func TestSupersededScanNeverReports(t *testing.T) {
	old, fresh := clips(t, 2), clips(t, 2)
	var oldStarted, oldDone atomic.Int32
	releaseOld, releaseFresh := make(chan struct{}), make(chan struct{})
	sc := NewScanner(probeFunc(func(_ context.Context, p string) (float64, error) { // ignores ctx on purpose
		if filepath.Dir(p) == old {
			oldStarted.Add(1)
			<-releaseOld
			oldDone.Add(1)
		} else {
			<-releaseFresh
		}
		return 1, nil
	}))
	got := make(chan lid.FolderSummary, 4)
	emit := func(fs lid.FolderSummary) { got <- fs }
	sc.Scan([]string{old}, mp3, emit)
	waitFor(t, "old probes in flight", func() bool { return oldStarted.Load() == 2 })
	sc.Scan([]string{fresh}, mp3, emit)

	close(releaseOld)
	waitFor(t, "old probes finished", func() bool { return oldDone.Load() == 2 })
	time.Sleep(50 * time.Millisecond) // time for a stale event to (wrongly) appear
	close(releaseFresh)
	if fs := recv(t, got); fs.Path != fresh {
		t.Fatalf("first event is for %s, want only %s", fs.Path, fresh)
	}
	select {
	case fs := <-got:
		t.Errorf("extra event %+v", fs)
	case <-time.After(50 * time.Millisecond):
	}
}

// The UI re-scans every folder on each list/setting change; unchanged files are not re-probed.
func TestScanDoesNotReprobeUnchangedFiles(t *testing.T) {
	root := clips(t, 4)
	var calls atomic.Int32
	sc := NewScanner(probeFunc(func(context.Context, string) (float64, error) { calls.Add(1); return 3, nil }))
	got := make(chan lid.FolderSummary, 4)
	emit := func(fs lid.FolderSummary) { got <- fs }

	sc.Scan([]string{root}, mp3, emit)
	recv(t, got)
	sc.Scan([]string{root}, mp3, emit)
	if fs := recv(t, got); fs.TotalDurationS == nil || *fs.TotalDurationS != 12 {
		t.Errorf("second total = %v, want 12", fs.TotalDurationS)
	}
	if n := calls.Load(); n != 4 {
		t.Errorf("prober called %d times for 4 files scanned twice", n)
	}
}
