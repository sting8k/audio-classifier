package engine

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
)

// Tests that need the bundled runtime read it from ENGINE_ASSETS, else third_party/<goos>-<goarch>
// (scripts/fetch-assets.sh), and skip when it is absent. Media is generated with the bundled
// ffmpeg; real speech comes from ENGINE_TEST_CLIPS (never committed).

func assetsConfig(t *testing.T) Config {
	t.Helper()
	dir := os.Getenv("ENGINE_ASSETS")
	if dir == "" {
		dir = filepath.Join("..", "..", "third_party", runtime.GOOS+"-"+runtime.GOARCH)
	}
	dir, _ = filepath.Abs(dir) // tests change directory
	cfg := AssetsIn(dir)
	if _, err := os.Stat(cfg.ORTLib); err != nil {
		t.Skipf("assets missing (run scripts/fetch-assets.sh): %v", err)
	}
	return cfg
}

func newEngine(t *testing.T, threads int) *Engine {
	t.Helper()
	cfg := assetsConfig(t)
	cfg.IntraOpThreads = threads
	e, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

// genMedia runs the bundled ffmpeg with args and returns the file it wrote into a temp dir.
func genMedia(t *testing.T, name string, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	args = append(append([]string{"-v", "error", "-y"}, args...), path)
	if out, err := exec.Command(assetsConfig(t).FFmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
	}
	return path
}

// The expected spans come from silero-vad 6.2.3's get_speech_timestamps_from_probs (Python)
// run on the same window probabilities.
func TestSpeechSpansMatchSilero(t *testing.T) {
	type run struct {
		p float32
		n int
	}
	build := func(runs ...run) (probs []float32) {
		for _, r := range runs {
			for range r.n {
				probs = append(probs, r.p)
			}
		}
		return probs
	}
	// Covers: speech at sample 0, a dip between the two thresholds, a 4-window silence that is
	// bridged, a 5-window silence that splits, a too-short blip that is dropped, and padding.
	a := build(run{.9, 10}, run{.4, 6}, run{.9, 5}, run{.1, 4}, run{.9, 6}, run{.1, 5}, run{.9, 9},
		run{.1, 30}, run{.9, 7}, run{.1, 30}, run{.9, 12}, run{.1, 5}, run{.9, 15}, run{.1, 5},
		run{.9, 8}, run{.2, 3}, run{.1, 2})
	b := build(run{.1, 20}, run{.9, 25}) // speech runs to the end of a partial last window
	cases := []struct {
		name  string
		probs []float32
		n     int
		want  []span
	}{
		{"mixed", a, len(a) * 512, []span{{0, 16352}, {17952, 23520}, {56864, 63968}, {65568, 74208}, {75808, 80864}}},
		{"speech to the end", b, len(b)*512 - 100, []span{{9760, 22940}}},
		{"no speech", build(run{.1, 50}), 25600, nil},
	}
	for _, c := range cases {
		if got := speechSpans(c.probs, c.n); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNaNLogitsAreAnError(t *testing.T) {
	if _, err := top2(softmax([]float32{float32(math.NaN()), 1, 2}), []string{"a", "b", "c"}); err == nil {
		t.Error("NaN logits produced a result")
	}
}

func TestSilentClipIsNoSpeech(t *testing.T) {
	e := newEngine(t, 1)
	// The name starts with '-' and holds ':' and spaces: it must reach ffmpeg as a plain file.
	path := genMedia(t, "-odd: name & ü.wav", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono", "-t", "5")
	t.Chdir(filepath.Dir(path))
	det, err := e.Detect(context.Background(), filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if !det.NoSpeech || len(det.Top) != 0 {
		t.Errorf("silent clip: %+v, want NoSpeech with no Top", det)
	}
	if math.Abs(det.DurationS-5) > 0.05 {
		t.Errorf("DurationS %v, want 5", det.DurationS)
	}
}

func TestUnreadableClipIsError(t *testing.T) {
	e := newEngine(t, 1)
	dir := t.TempDir()
	garbage := filepath.Join(dir, "notes.mp3")
	os.WriteFile(garbage, []byte("this is not audio"), 0o644)
	videoOnly := genMedia(t, "video.mp4", "-f", "lavfi", "-i", "testsrc=d=2:s=64x64:r=10")
	for name, path := range map[string]string{"garbage": garbage, "no audio stream": videoOnly, "missing": filepath.Join(dir, "gone.wav")} {
		if det, err := e.Detect(context.Background(), path); err == nil {
			t.Errorf("%s: got %+v, want an error", name, det)
		}
	}
}

// SPEC D-1: only the first 30 s are analysed, yet DurationS is the whole clip.
func TestLongClipIsCappedButDurationIsFull(t *testing.T) {
	e := newEngine(t, 1)
	path := genMedia(t, "long.wav", "-f", "lavfi", "-i", "sine=f=300:r=16000", "-t", "45")
	pcm, durS, err := e.decode(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != maxSamples || math.Abs(durS-45) > 0.05 {
		t.Errorf("decoded %d samples (want %d), duration %v (want 45)", len(pcm), maxSamples, durS)
	}
	if d, err := e.Duration(context.Background(), path); err != nil || math.Abs(d-45) > 0.05 {
		t.Errorf("Duration = %v, %v; want 45", d, err)
	}
}

func TestCanceledContextStopsDetect(t *testing.T) {
	e := newEngine(t, 1)
	path := genMedia(t, "a.wav", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono", "-t", "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Detect(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("Detect err = %v, want context.Canceled", err)
	}
	if _, err := e.Duration(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("Duration err = %v, want context.Canceled", err)
	}
}

// ORT's environment is process-wide and shared by refcount: closing one Engine must not pull
// it from under another.
func TestClosingOneEngineKeepsOthersAlive(t *testing.T) {
	cfg := assetsConfig(t)
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	a.Close()
	if _, err := b.classify(make([]float32, sampleRate)); err != nil {
		t.Fatal(err)
	}
}

// Many goroutines share one Engine (one ECAPA session, one VAD session) under -race, and the
// answers do not depend on who ran concurrently. Noise goes through the model directly
// (the VAD would drop it); real clips go through Detect when ENGINE_TEST_CLIPS is set.
func TestConcurrentUseGivesSameAnswers(t *testing.T) {
	e := newEngine(t, 2)
	rng := rand.New(rand.NewPCG(1, 2))
	noise := make([]float32, 5*sampleRate)
	for i := range noise {
		noise[i] = float32(rng.NormFloat64()) * 0.1
	}
	want, err := e.classify(noise)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := e.classify(noise)
			if err != nil || got[0].Lang != want[0].Lang || math.Abs(got[0].Prob-want[0].Prob) > 1e-5 {
				t.Errorf("concurrent classify = %v, %v; want %v", got, err, want)
			}
		}()
	}
	wg.Wait()
}

// Real speech: top-1 must be right and probabilities close to the Python reference
// (same ONNX files; with the same decoded PCM Python gives id .9968, pt 1.0000, ar .9521).
func TestReferenceClips(t *testing.T) {
	dir := os.Getenv("ENGINE_TEST_CLIPS")
	if dir == "" {
		t.Skip("ENGINE_TEST_CLIPS not set")
	}
	e := newEngine(t, 2)
	ref := []struct {
		glob, lang string
		prob       float64
	}{{"Live Earth*", "id", .9968}, {"PT*", "pt", 1.0}, {"arabic 5*", "ar", .9521}}
	for _, r := range ref {
		files, _ := filepath.Glob(filepath.Join(dir, r.glob))
		if len(files) != 1 {
			t.Fatalf("%s: found %v", r.glob, files)
		}
		var wg sync.WaitGroup // the same clip from several goroutines at once
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				det, err := e.Detect(context.Background(), files[0])
				if err != nil || det.NoSpeech || len(det.Top) != 2 {
					t.Errorf("%s: %+v, %v", r.glob, det, err)
					return
				}
				if det.Top[0].Lang != r.lang || math.Abs(det.Top[0].Prob-r.prob) > 0.005 {
					t.Errorf("%s: top-1 %v, want %s %.4f", r.glob, det.Top[0], r.lang, r.prob)
				}
				if det.Top[1].Prob > det.Top[0].Prob || det.Top[1].Lang == det.Top[0].Lang {
					t.Errorf("%s: top-2 not below top-1: %v", r.glob, det.Top)
				}
			}()
		}
		wg.Wait()
	}
}
