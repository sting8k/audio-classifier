package batch

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// DefaultInterval is the progress snapshot period while a run is active
// (a UI refresh rate, not a measured value).
const DefaultInterval = 250 * time.Millisecond

// recentMax is how many newest results a Progress snapshot carries.
const recentMax = 5

// autoWorkers is the pool size when Settings.Workers is 0: one per core. In the
// engine benchmark (6 cores, 1 ORT thread per clip) 6 workers were fastest and 5
// within noise; per-worker speed drops as workers grow, so this is not linear.
func autoWorkers() int { return runtime.NumCPU() }

var errBusy = errors.New("a run is already in progress")

// Config wires a Runner to its engine and to the UI.
type Config struct {
	Detector lid.Detector
	// Prober is optional; it fills ClipResult.DurationS when the Detection
	// does not carry the clip duration.
	Prober lid.Prober
	// OnProgress receives a snapshot every Interval while a run is active and
	// immediately on every state change (start, pause, resume, cancel, done).
	// Calls are serialised and ordered. Optional.
	OnProgress func(lid.Progress)
	Interval   time.Duration // 0 = DefaultInterval
}

// Runner owns at most one active run and keeps the last run's results.
type Runner struct {
	cfg Config
	now func() time.Time // tests replace it

	mu  sync.Mutex
	cur *run
}

func NewRunner(cfg Config) *Runner {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	return &Runner{cfg: cfg, now: time.Now}
}

// Start scans the folders and runs every matching clip. It returns once the
// run is under way; follow it with Progress/OnProgress. A run that is still
// running or paused must be cancelled first.
func (r *Runner) Start(paths []string, s lid.Settings) error {
	var folders []Folder
	seen := map[string]bool{}
	for _, p := range paths {
		if k := filepath.Clean(p); !seen[k] {
			seen[k] = true
			folders = append(folders, ScanFolder(p, s))
		}
	}
	total := 0
	for _, f := range folders {
		total += len(f.Files)
	}
	if total == 0 {
		return errNoClips
	}

	r.mu.Lock()
	if r.cur != nil && r.cur.active() {
		r.mu.Unlock()
		return errBusy
	}
	nr := newRun(r.cfg, r.now, folders, s)
	r.cur = nr
	r.mu.Unlock()
	nr.start() // outside the lock: it calls OnProgress
	return nil
}

func (r *Runner) current() *run {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cur
}

// Pause stops dispatching new clips; clips already in flight finish.
func (r *Runner) Pause() {
	if c := r.current(); c != nil {
		c.pause()
	}
}

func (r *Runner) Resume() {
	if c := r.current(); c != nil {
		c.resume()
	}
}

// Cancel stops the run now: in-flight clips are interrupted and whatever
// finished before stays in Results.
func (r *Runner) Cancel() {
	if c := r.current(); c != nil {
		c.stopNow()
	}
}

// Shutdown cancels the current run and waits up to grace for its workers to
// leave the Detector, so the caller can Close it without pulling it out from
// under a running Detect.
func (r *Runner) Shutdown(grace time.Duration) {
	c := r.current()
	if c == nil {
		return
	}
	c.stopNow()
	select {
	case <-c.exited:
	case <-time.After(grace):
	}
}

// Progress is the current snapshot; idle if nothing ever ran.
func (r *Runner) Progress() lid.Progress {
	c := r.current()
	if c == nil {
		return lid.Progress{State: lid.StateIdle, Folders: []lid.FolderProgress{}, Recent: []lid.ClipResult{}}
	}
	return c.progress()
}

// Results are the finished clips of the current/last run in scan order
// (folder order, then walk order), whatever order they completed in.
func (r *Runner) Results() []lid.ClipResult {
	c := r.current()
	if c == nil {
		return []lid.ClipResult{}
	}
	return c.finished()
}

// Settings are the settings of the current/last run (zero if none).
func (r *Runner) Settings() lid.Settings {
	if c := r.current(); c != nil {
		return c.settings
	}
	return lid.Settings{}
}

type job struct {
	folder int    // index into run.folders
	rel    string // File of the result
	path   string
}

type run struct {
	cfg      Config
	now      func() time.Time
	settings lid.Settings
	folders  []Folder
	jobs     []job
	ctx      context.Context
	cancel   context.CancelFunc
	exited   chan struct{} // closed when every worker has returned

	mu        sync.Mutex
	cond      *sync.Cond // signalled on resume and cancel
	state     string
	next      int
	inflight  map[int]struct{}
	results   []*lid.ClipResult // by job index; nil until finished
	done      int
	perFolder []int
	summary   lid.Summary
	recent    []lid.ClipResult // newest first, <= recentMax
	started   time.Time
	pausedAt  time.Time
	pausedFor time.Duration
	endedAt   time.Time

	emitMu sync.Mutex // orders snapshots: later emit => later state
}

func newRun(cfg Config, now func() time.Time, folders []Folder, s lid.Settings) *run {
	r := &run{cfg: cfg, now: now, settings: s, folders: folders, exited: make(chan struct{}),
		state: lid.StateRunning, inflight: map[int]struct{}{}, perFolder: make([]int, len(folders))}
	r.cond = sync.NewCond(&r.mu)
	r.ctx, r.cancel = context.WithCancel(context.Background())
	for fi, f := range folders {
		for _, rel := range f.Files {
			r.jobs = append(r.jobs, job{folder: fi, rel: rel, path: f.abs(rel)})
		}
	}
	r.results = make([]*lid.ClipResult, len(r.jobs))
	r.started = now()
	return r
}

func (r *run) start() {
	n := r.settings.Workers
	if n <= 0 {
		n = autoWorkers()
	}
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker()
		}()
	}
	go r.ticker()
	go func() {
		wg.Wait()
		r.mu.Lock()
		natural := r.state != lid.StateCancelled
		if natural {
			r.endLocked(lid.StateDone)
		}
		r.mu.Unlock()
		if natural {
			r.emit()
		}
		r.cancel() // releases the ticker
		close(r.exited)
	}()
	r.emit()
}

func (r *run) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == lid.StateRunning || r.state == lid.StatePaused
}

// take hands out the next clip. Once pause() returned, no further clip is
// handed out until resume(); this is what makes "pause stops dispatch" exact.
func (r *run) take() (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if r.state == lid.StateCancelled || r.next >= len(r.jobs) {
			return 0, false
		}
		if r.state != lid.StatePaused {
			break
		}
		r.cond.Wait()
	}
	i := r.next
	r.next++
	r.inflight[i] = struct{}{}
	return i, true
}

func (r *run) worker() {
	for {
		i, ok := r.take()
		if !ok {
			return
		}
		r.record(i, r.process(r.jobs[i]))
	}
}

// process classifies one clip. After Cancel the Detector fails with the
// cancelled context; record drops that row along with any other late one.
func (r *run) process(j job) (res lid.ClipResult) {
	res = lid.ClipResult{Folder: r.folders[j.folder].Dir, File: j.rel, Path: j.path}
	det, err := r.cfg.Detector.Detect(r.ctx, j.path)
	if err != nil {
		res.Status, res.Error = lid.StatusError, err.Error()
		return res
	}
	dur := det.DurationS
	if dur <= 0 && r.cfg.Prober != nil {
		if d, err := r.cfg.Prober.Duration(r.ctx, j.path); err == nil {
			dur = d
		}
	}
	if dur > 0 {
		res.DurationS = &dur
	}
	speech := det.SpeechS
	res.SpeechS = &speech
	switch {
	case det.NoSpeech:
		res.Status = lid.StatusNoSpeech
	case len(det.Top) == 0: // contract says never; don't index into nothing
		res.Status, res.Error = lid.StatusError, "engine returned no language"
	default:
		top := det.Top[0]
		conf := top.Prob
		res.Language, res.Confidence = top.Lang, &conf
		if len(det.Top) > 1 {
			second := det.Top[1].Prob
			res.SecondLanguage, res.SecondConfidence = det.Top[1].Lang, &second
		}
		res.Status = lid.StatusReview
		if margin := conf - secondProb(det.Top); margin >= r.settings.MinMargin {
			res.Status = lid.StatusOK
		}
	}
	return res
}

// secondProb is the runner-up's probability, 0 when the engine gave only one class.
func secondProb(top []lid.LangProb) float64 {
	if len(top) > 1 {
		return top[1].Prob
	}
	return 0
}

func (r *run) record(i int, res lid.ClipResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inflight, i)
	if r.state == lid.StateCancelled { // results landing after Cancel are dropped
		return
	}
	r.results[i] = &res
	r.done++
	r.perFolder[r.jobs[i].folder]++
	r.summary.Total++
	switch res.Status {
	case lid.StatusOK:
		r.summary.OK++
	case lid.StatusReview:
		r.summary.Review++
	case lid.StatusNoSpeech:
		r.summary.NoSpeech++
	case lid.StatusError:
		r.summary.Error++
	}
	r.recent = append([]lid.ClipResult{res}, r.recent...)
	if len(r.recent) > recentMax {
		r.recent = r.recent[:recentMax]
	}
}

func (r *run) pause() {
	r.mu.Lock()
	if r.state != lid.StateRunning {
		r.mu.Unlock()
		return
	}
	r.state, r.pausedAt = lid.StatePaused, r.now()
	r.mu.Unlock()
	r.emit()
}

func (r *run) resume() {
	r.mu.Lock()
	if r.state != lid.StatePaused {
		r.mu.Unlock()
		return
	}
	r.pausedFor += r.now().Sub(r.pausedAt)
	r.state = lid.StateRunning
	r.cond.Broadcast()
	r.mu.Unlock()
	r.emit()
}

func (r *run) stopNow() {
	r.mu.Lock()
	if r.state != lid.StateRunning && r.state != lid.StatePaused {
		r.mu.Unlock()
		return
	}
	r.endLocked(lid.StateCancelled)
	r.cond.Broadcast()
	r.cancel()
	r.mu.Unlock()
	r.emit()
}

// endLocked moves to a terminal state and freezes the clock.
func (r *run) endLocked(state string) {
	if r.state == lid.StatePaused {
		r.endedAt = r.pausedAt
	} else {
		r.endedAt = r.now()
	}
	r.state = state
}

// elapsedLocked is active time: time spent paused does not count.
func (r *run) elapsedLocked() time.Duration {
	end := r.now()
	switch r.state {
	case lid.StatePaused:
		end = r.pausedAt
	case lid.StateDone, lid.StateCancelled:
		end = r.endedAt
	}
	return end.Sub(r.started) - r.pausedFor
}

func (r *run) progress() lid.Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(r.jobs)
	p := lid.Progress{State: r.state, Done: r.done, Total: total, Summary: r.summary,
		Folders: make([]lid.FolderProgress, len(r.folders)),
		Recent:  append(make([]lid.ClipResult, 0, len(r.recent)), r.recent...)}
	el := r.elapsedLocked().Seconds()
	p.ElapsedS = el
	if (r.state == lid.StateRunning || r.state == lid.StatePaused) && r.done > 0 {
		eta := el / float64(r.done) * float64(total-r.done)
		p.EtaS = &eta
	}
	first := -1
	for i := range r.inflight {
		if first < 0 || i < first {
			first = i
		}
	}
	if first >= 0 {
		p.Current = r.jobs[first].path
	}
	for i, f := range r.folders {
		p.Folders[i] = lid.FolderProgress{Path: f.Path, Name: f.Name, Done: r.perFolder[i], Total: len(f.Files)}
	}
	return p
}

func (r *run) finished() []lid.ClipResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]lid.ClipResult, 0, r.done)
	for _, c := range r.results {
		if c != nil {
			out = append(out, *c)
		}
	}
	return out
}

// emit delivers one snapshot. The snapshot is taken under emitMu, so
// deliveries are ordered: nothing stale can follow a terminal state.
func (r *run) emit() {
	if r.cfg.OnProgress == nil {
		return
	}
	r.emitMu.Lock()
	defer r.emitMu.Unlock()
	r.cfg.OnProgress(r.progress())
}

func (r *run) ticker() {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-t.C:
			r.emit()
		}
	}
}
