package batch

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// Statuses come from the user's MinMargin (top-1 minus top-2), inclusive at the boundary;
// a high top-1 with a close runner-up still needs review.
func TestStatusFollowsMargin(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "clear.mp3", "edge.mp3", "close.mp3")
	top := map[string][2]float64{"clear.mp3": {0.80, 0.10}, "edge.mp3": {0.75, 0.50}, "close.mp3": {0.55, 0.45}}
	det := detFunc(func(_ context.Context, p string) (lid.Detection, error) {
		pr := top[filepath.Base(p)]
		return lid.Detection{DurationS: 10, SpeechS: 8, Top: []lid.LangProb{{Lang: "vi", Prob: pr[0]}, {Lang: "en", Prob: pr[1]}}}, nil
	})

	for _, tc := range []struct {
		min  float64
		want map[string]lid.Status
	}{
		{0.25, map[string]lid.Status{"clear.mp3": lid.StatusOK, "edge.mp3": lid.StatusOK, "close.mp3": lid.StatusReview}},
		{0.50, map[string]lid.Status{"clear.mp3": lid.StatusOK, "edge.mp3": lid.StatusReview, "close.mp3": lid.StatusReview}},
	} {
		r := NewRunner(Config{Detector: det})
		s := mp3
		s.MinMargin = tc.min
		if err := r.Start([]string{root}, s); err != nil {
			t.Fatal(err)
		}
		r.waitExited(t)
		for _, c := range r.Results() {
			if c.Status != tc.want[c.File] {
				t.Errorf("min %.2f: %s = %s, want %s", tc.min, c.File, c.Status, tc.want[c.File])
			}
		}
	}
}

// Every outcome of Detect maps to a row the UI can show; none is lost.
func TestOutcomesBecomeRows(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "bad.mp3", "mute.mp3", "ok.mp3", "unprobed.mp3")
	det := detFunc(func(_ context.Context, p string) (lid.Detection, error) {
		switch filepath.Base(p) {
		case "bad.mp3":
			return lid.Detection{}, context.DeadlineExceeded // any error that is not our cancel
		case "mute.mp3":
			return lid.Detection{DurationS: 7, NoSpeech: true}, nil
		case "unprobed.mp3":
			d := speech("th", 0.9)
			d.DurationS = 0 // engine did not know the duration
			return d, nil
		}
		return speech("vi", 0.9), nil
	})
	prober := probeFunc(func(context.Context, string) (float64, error) { return 42, nil })
	r := NewRunner(Config{Detector: det, Prober: prober})
	if err := r.Start([]string{root}, mp3); err != nil {
		t.Fatal(err)
	}
	r.waitExited(t)

	got := map[string]lid.ClipResult{}
	for _, c := range r.Results() {
		got[c.File] = c
	}
	if c := got["bad.mp3"]; c.Status != lid.StatusError || c.Error == "" || c.Language != "" || c.DurationS != nil {
		t.Errorf("bad = %+v, want error with message and no language/duration", c)
	}
	if c := got["mute.mp3"]; c.Status != lid.StatusNoSpeech || c.Language != "" || c.Confidence != nil || c.DurationS == nil || *c.DurationS != 7 {
		t.Errorf("mute = %+v, want no_speech, no language, duration 7", c)
	}
	if c := got["ok.mp3"]; c.Language != "vi" || c.SecondLanguage != "en" || c.SecondConfidence == nil || *c.DurationS != 10 {
		t.Errorf("ok = %+v, want vi/en and the detector's duration", c)
	}
	if c := got["unprobed.mp3"]; c.DurationS == nil || *c.DurationS != 42 {
		t.Errorf("unprobed = %+v, want the prober's duration", c)
	}
	if s := r.Progress().Summary; s != (lid.Summary{Total: 4, OK: 2, NoSpeech: 1, Error: 1}) {
		t.Errorf("summary = %+v", s)
	}
}

// The pool never exceeds Settings.Workers, and really uses them.
func TestWorkerPoolIsBounded(t *testing.T) {
	const workers = 3
	var inside, peak atomic.Int32
	reached := make(chan struct{})
	det := detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		n := inside.Add(1)
		defer inside.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		if n == workers {
			select {
			case <-reached:
			default:
				close(reached)
			}
		}
		select { // hold the first wave until `workers` clips are in flight at once
		case <-reached:
		case <-time.After(5 * time.Second):
		}
		time.Sleep(5 * time.Millisecond)
		return speech("vi", 0.9), nil
	})
	r := NewRunner(Config{Detector: det})
	s := mp3
	s.Workers = workers
	if err := r.Start([]string{clips(t, 12)}, s); err != nil {
		t.Fatal(err)
	}
	r.waitExited(t)
	if got := peak.Load(); got != workers {
		t.Errorf("peak concurrency = %d, want %d", got, workers)
	}
	if d := r.Progress().Done; d != 12 {
		t.Errorf("done = %d, want 12", d)
	}
}

// Cancel returns at once, keeps what finished, and drops anything that lands
// afterwards, including from a detector that ignores its context.
func TestCancelKeepsPartialResults(t *testing.T) {
	lateRelease := make(chan struct{})
	var started atomic.Int32
	det := detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		started.Add(1)
		switch filepath.Base(p) {
		case "c00.mp3", "c01.mp3":
			return speech("vi", 0.9), nil
		case "c03.mp3": // ignores ctx, finishes after Cancel
			<-lateRelease
			return speech("vi", 0.9), nil
		}
		<-ctx.Done()
		return lid.Detection{}, ctx.Err()
	})
	r := NewRunner(Config{Detector: det})
	s := mp3
	s.Workers = 2
	if err := r.Start([]string{clips(t, 10)}, s); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "two done, two blocked", func() bool { return r.Progress().Done == 2 && started.Load() == 4 })

	t0 := time.Now()
	r.Cancel()
	if d := time.Since(t0); d > time.Second {
		t.Errorf("Cancel took %v", d)
	}
	if p := r.Progress(); p.State != lid.StateCancelled || p.Done != 2 {
		t.Errorf("after cancel: state=%s done=%d, want cancelled/2", p.State, p.Done)
	}
	close(lateRelease)
	r.waitExited(t)

	res := r.Results()
	if len(res) != 2 || res[0].File != "c00.mp3" || res[1].File != "c01.mp3" {
		t.Errorf("results = %+v, want exactly the 2 clips finished before Cancel", res)
	}
	if p := r.Progress(); p.State != lid.StateCancelled || p.Summary.Error != 0 || started.Load() != 4 {
		t.Errorf("after exit: state=%s errors=%d started=%d; interrupted clips must not become error rows, nothing new may start", p.State, p.Summary.Error, started.Load())
	}
}

// Once Shutdown returns no worker is inside the Detector, so closing it is safe.
func TestShutdownWaitsForWorkersToLeave(t *testing.T) {
	var inside atomic.Int32
	r := NewRunner(Config{Detector: detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		inside.Add(1)
		defer inside.Add(-1)
		<-ctx.Done()
		time.Sleep(30 * time.Millisecond) // winding down takes a moment
		return lid.Detection{}, ctx.Err()
	})})
	s := mp3
	s.Workers = 2
	if err := r.Start([]string{clips(t, 4)}, s); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "workers inside the detector", func() bool { return inside.Load() == 2 })
	r.Shutdown(5 * time.Second)
	if n := inside.Load(); n != 0 {
		t.Errorf("%d workers still inside Detect after Shutdown", n)
	}
}

// Pause: nothing new is dispatched, in-flight clips finish; Resume completes the run.
func TestPauseStopsDispatchResumeFinishes(t *testing.T) {
	gate := make(chan struct{})
	var started atomic.Int32
	det := detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		started.Add(1)
		select {
		case <-gate:
			return speech("vi", 0.9), nil
		case <-ctx.Done():
			return lid.Detection{}, ctx.Err()
		}
	})
	r := NewRunner(Config{Detector: det})
	s := mp3
	s.Workers = 2
	if err := r.Start([]string{clips(t, 8)}, s); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "2 in flight", func() bool { return started.Load() == 2 })

	r.Pause()
	gate <- struct{}{} // both in-flight clips finish while paused
	gate <- struct{}{}
	waitFor(t, "in-flight clips recorded", func() bool { return r.Progress().Done == 2 })
	time.Sleep(50 * time.Millisecond) // a leaking worker would have started a 3rd clip by now
	if p := r.Progress(); p.State != lid.StatePaused || started.Load() != 2 || p.Current != "" {
		t.Fatalf("while paused: state=%s started=%d current=%q, want paused/2/none", p.State, started.Load(), p.Current)
	}

	r.Resume()
	close(gate)
	r.waitExited(t)
	if p := r.Progress(); p.State != lid.StateDone || p.Done != 8 || len(r.Results()) != 8 {
		t.Errorf("after resume: state=%s done=%d results=%d, want done/8/8", p.State, p.Done, len(r.Results()))
	}
}

// Snapshots are throttled (not one per clip), the last one always arrives,
// and nothing stale follows it.
func TestProgressSnapshots(t *testing.T) {
	a, b := clips(t, 3), clips(t, 8)
	var mu sync.Mutex
	var events []lid.Progress
	r := NewRunner(Config{
		Detector:   detFunc(func(context.Context, string) (lid.Detection, error) { return speech("vi", 0.9), nil }),
		Interval:   time.Hour, // only state changes may emit
		OnProgress: func(p lid.Progress) { mu.Lock(); events = append(events, p); mu.Unlock() },
	})
	s := mp3
	s.Workers = 1 // completion order = scan order
	if err := r.Start([]string{a, b}, s); err != nil {
		t.Fatal(err)
	}
	r.waitExited(t)

	mu.Lock()
	defer mu.Unlock()
	if len(events) >= 11 {
		t.Errorf("%d events for 11 clips: not throttled", len(events))
	}
	last := events[len(events)-1]
	if last.State != lid.StateDone || last.Done != 11 || last.Total != 11 {
		t.Fatalf("last event = %s %d/%d, want done 11/11", last.State, last.Done, last.Total)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Done < events[i-1].Done || (events[i-1].State == lid.StateDone && events[i].State != lid.StateDone) {
			t.Errorf("event %d went backwards: %s %d after %s %d", i, events[i].State, events[i].Done, events[i-1].State, events[i-1].Done)
		}
	}
	if len(last.Folders) != 2 || last.Folders[0].Done != 3 || last.Folders[0].Total != 3 || last.Folders[1].Done != 8 || last.Folders[1].Total != 8 {
		t.Errorf("folders = %+v", last.Folders)
	}
	if len(last.Recent) != 5 || last.Recent[0].File != "c07.mp3" || last.Recent[4].File != "c03.mp3" || last.Recent[0].Folder != b {
		t.Errorf("recent = %v, want the 5 newest, newest first", files(last.Recent))
	}
	if last.Current != "" || last.EtaS != nil {
		t.Errorf("finished run: current=%q eta=%v, want none", last.Current, last.EtaS)
	}
}

func files(rs []lid.ClipResult) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.File)
	}
	return out
}

// A long clip must not freeze the elapsed clock on screen 2.
func TestProgressTicksWhileClipIsSlow(t *testing.T) {
	var n atomic.Int32
	r := NewRunner(Config{
		Detector: detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
			<-ctx.Done()
			return lid.Detection{}, ctx.Err()
		}),
		Interval:   5 * time.Millisecond,
		OnProgress: func(lid.Progress) { n.Add(1) },
	})
	if err := r.Start([]string{clips(t, 1)}, mp3); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "periodic snapshots with no clip finishing", func() bool { return n.Load() >= 4 })
	r.Cancel()
}

// Elapsed and ETA count active time only; pausing must not inflate them.
func TestElapsedAndETAExcludePause(t *testing.T) {
	clk := &clock{t: time.Unix(1000, 0)}
	gate := make(chan struct{})
	r := NewRunner(Config{Detector: detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		select {
		case <-gate:
			return speech("vi", 0.9), nil
		case <-ctx.Done():
			return lid.Detection{}, ctx.Err()
		}
	})})
	r.now = clk.now
	s := mp3
	s.Workers = 1
	if err := r.Start([]string{clips(t, 3)}, s); err != nil {
		t.Fatal(err)
	}
	if p := r.Progress(); p.EtaS != nil {
		t.Errorf("eta before the first result = %v, want nil", *p.EtaS)
	}

	clk.advance(10 * time.Second)
	gate <- struct{}{}
	waitFor(t, "first result", func() bool { return r.Progress().Done == 1 })
	if p := r.Progress(); p.ElapsedS != 10 || p.EtaS == nil || *p.EtaS != 20 {
		t.Errorf("after 1/3 in 10s: elapsed=%v eta=%v, want 10 and 20", p.ElapsedS, p.EtaS)
	}

	r.Pause()
	clk.advance(100 * time.Second)
	if p := r.Progress(); p.ElapsedS != 10 {
		t.Errorf("paused elapsed = %v, want 10", p.ElapsedS)
	}
	r.Resume()
	clk.advance(5 * time.Second)
	if p := r.Progress(); p.ElapsedS != 15 {
		t.Errorf("resumed elapsed = %v, want 15 (the 100s pause excluded)", p.ElapsedS)
	}

	close(gate)
	r.waitExited(t)
	clk.advance(time.Hour)
	if p := r.Progress(); p.State != lid.StateDone || p.ElapsedS != 15 {
		t.Errorf("finished: state=%s elapsed=%v, want done/15 frozen", p.State, p.ElapsedS)
	}
}

// One run at a time; a start with nothing to do is refused and keeps the old results.
func TestStartGuards(t *testing.T) {
	r := NewRunner(Config{Detector: detFunc(func(ctx context.Context, p string) (lid.Detection, error) {
		<-ctx.Done()
		return lid.Detection{}, ctx.Err()
	})})
	if err := r.Start([]string{t.TempDir()}, mp3); err == nil {
		t.Error("start on a folder without clips succeeded")
	}
	if r.Progress().State != lid.StateIdle {
		t.Error("a refused start must leave the runner idle")
	}
	root := clips(t, 2)
	if err := r.Start([]string{root, root + "/"}, mp3); err != nil { // same folder twice
		t.Fatal(err)
	}
	if got := r.Progress().Total; got != 2 {
		t.Errorf("total = %d, want 2: a folder listed twice runs once", got)
	}
	if err := r.Start([]string{root}, mp3); err == nil {
		t.Error("second start while running succeeded")
	}
	r.Pause()
	if err := r.Start([]string{root}, mp3); err == nil {
		t.Error("start while paused succeeded")
	}
	r.Cancel()
	r.waitExited(t)
	if err := r.Start([]string{root}, mp3); err != nil {
		t.Errorf("start after cancel: %v", err)
	}
	r.Cancel()
}
