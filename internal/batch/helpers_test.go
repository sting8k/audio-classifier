package batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// detFunc / probeFunc adapt closures to the lid interfaces.
type detFunc func(ctx context.Context, path string) (lid.Detection, error)

func (f detFunc) Detect(ctx context.Context, p string) (lid.Detection, error) { return f(ctx, p) }
func (detFunc) Close() error                                                  { return nil }

type probeFunc func(ctx context.Context, path string) (float64, error)

func (f probeFunc) Duration(ctx context.Context, p string) (float64, error) { return f(ctx, p) }

// speech is a successful Detection: language lang with probability p.
func speech(lang string, p float64) lid.Detection {
	return lid.Detection{DurationS: 10, SpeechS: 8, Top: []lid.LangProb{{Lang: lang, Prob: p}, {Lang: "en", Prob: (1 - p) / 2}}}
}

// touch creates empty files (and parent dirs) under root.
func touch(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(root, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// clips makes a folder with n files c00.mp3, c01.mp3, ...
func clips(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := range n {
		touch(t, root, fmt.Sprintf("c%02d.mp3", i))
	}
	return root
}

var mp3 = lid.Settings{Extensions: []string{"mp3"}, MinConfidence: 0.6}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (r *Runner) waitExited(t *testing.T) {
	t.Helper()
	select {
	case <-r.current().exited:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not exit")
	}
}

// clock is a hand-wound time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
