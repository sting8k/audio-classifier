package batch

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// Scanner answers ScanFolders: it walks the folders and returns the counts at
// once, then probes clip durations in the background and reports each folder's
// total through onTotals when that folder is complete. Probing one file may
// spawn a process, so it must never sit on the caller's path.
type Scanner struct {
	prober lid.Prober // nil: totals are never reported

	mu    sync.Mutex // guards gen, stop, cache; held while onTotals runs
	gen   uint64
	stop  context.CancelFunc
	cache map[string]probed
}

// probed is a duration remembered for one file version; the UI re-scans every
// folder each time the list or settings change.
type probed struct {
	mod  time.Time
	size int64
	dur  float64
}

func NewScanner(p lid.Prober) *Scanner {
	return &Scanner{prober: p, cache: map[string]probed{}}
}

// probeWorkers bounds concurrent probes (a guess: probes are short processes).
func probeWorkers() int { return min(max(runtime.NumCPU(), 2), 8) }

// Scan returns one summary per path, in order, without waiting for durations.
// It supersedes any earlier Scan: that one's probing stops and its onTotals
// is never called again once this call has returned.
func (s *Scanner) Scan(paths []string, set lid.Settings, onTotals func(lid.FolderSummary)) []lid.FolderSummary {
	ctx, gen := s.supersede()
	folders := make([]Folder, len(paths))
	sums := make([]lid.FolderSummary, len(paths))
	for i, p := range paths {
		folders[i] = ScanFolder(p, set)
		sums[i] = folders[i].Summary()
	}
	if s.prober != nil && onTotals != nil {
		go s.probeAll(ctx, gen, folders, sums, onTotals)
	}
	return sums
}

// Cancel stops background probing (a run is starting and needs the CPU).
func (s *Scanner) Cancel() { s.supersede() }

func (s *Scanner) supersede() (context.Context, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop()
	}
	var ctx context.Context
	ctx, s.stop = context.WithCancel(context.Background())
	s.gen++
	return ctx, s.gen
}

func (s *Scanner) probeAll(ctx context.Context, gen uint64, folders []Folder, sums []lid.FolderSummary, onTotals func(lid.FolderSummary)) {
	type item struct {
		folder int
		path   string
	}
	var items []item
	pending := make([]int, len(folders))
	total := make([]float64, len(folders))
	for fi, f := range folders {
		pending[fi] = len(f.Files)
		for _, rel := range f.Files {
			items = append(items, item{fi, f.abs(rel)})
		}
	}

	var pmu sync.Mutex // guards pending, total
	finish := func(it item, d float64, ok bool) {
		pmu.Lock()
		if ok {
			total[it.folder] += d
		}
		pending[it.folder]--
		last, sum := pending[it.folder] == 0, total[it.folder]
		pmu.Unlock()
		if !last {
			return
		}
		out := sums[it.folder]
		out.TotalDurationS = &sum
		s.mu.Lock()
		if s.gen == gen { // still the latest scan
			onTotals(out)
		}
		s.mu.Unlock()
	}

	ch := make(chan item)
	var wg sync.WaitGroup
	for range probeWorkers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				d, ok := s.duration(ctx, it.path)
				finish(it, d, ok)
			}
		}()
	}
feed:
	for _, it := range items {
		select {
		case ch <- it:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
}

func (s *Scanner) duration(ctx context.Context, path string) (float64, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	s.mu.Lock()
	c, hit := s.cache[path]
	s.mu.Unlock()
	if hit && c.size == st.Size() && c.mod.Equal(st.ModTime()) {
		return c.dur, true
	}
	d, err := s.prober.Duration(ctx, path)
	if err != nil {
		return 0, false
	}
	s.mu.Lock()
	s.cache[path] = probed{mod: st.ModTime(), size: st.Size(), dur: d}
	s.mu.Unlock()
	return d, true
}
