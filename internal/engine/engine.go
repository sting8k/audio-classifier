// Package engine is the real lid.Detector / lid.Prober:
// ffmpeg decode (first 30 s) -> Silero VAD -> one ECAPA pass over the kept speech -> top 2.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/sting8k/audio-classifier/internal/lid"
	ort "github.com/yalue/onnxruntime_go"
)

const sampleRate = 16000

// Config holds explicit file locations (no guessing inside the package) and thread settings.
type Config struct {
	FFmpeg  string // ffmpeg executable; it also does the duration probe, so no ffprobe is needed
	ORTLib  string // onnxruntime shared library (must be ORT 1.23.x, see scripts/fetch-assets.sh)
	ECAPA   string // voxlingua107.onnx
	LangMap string // lang_map.json shipped with the ECAPA model
	VAD     string // silero_vad.onnx

	// IntraOpThreads is the thread count of the ECAPA session's intra-op pool, shared by all
	// concurrent Detect calls. 0 lets ORT use one thread per core.
	IntraOpThreads int
}

// Engine implements lid.Detector and lid.Prober. Safe for concurrent use.
type Engine struct {
	ffmpeg string
	langs  []string // ECAPA class index -> ISO code
	vad    *ort.DynamicAdvancedSession
	ecapa  *ort.DynamicAdvancedSession
}

var (
	_ lid.Detector = (*Engine)(nil)
	_ lid.Prober   = (*Engine)(nil)
)

// ORT keeps one process-wide environment; every Engine holds a reference to it so that
// New after Close (e.g. when the user changes thread settings) works.
var env struct {
	mu   sync.Mutex
	refs int
	lib  string
}

func acquireEnv(lib string) error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.refs > 0 {
		if env.lib != lib {
			return fmt.Errorf("onnxruntime already loaded from %s", env.lib)
		}
		env.refs++
		return nil
	}
	ort.SetSharedLibraryPath(lib)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("onnxruntime: %w", err)
	}
	env.lib, env.refs = lib, 1
	return nil
}

func releaseEnv() {
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.refs--; env.refs == 0 {
		ort.DestroyEnvironment()
	}
}

// New loads both models. Close must be called to free them.
func New(cfg Config) (*Engine, error) {
	for _, p := range []string{cfg.FFmpeg, cfg.ORTLib, cfg.ECAPA, cfg.LangMap, cfg.VAD} {
		if _, err := os.Stat(p); err != nil {
			return nil, err
		}
	}
	langs, err := loadLangMap(cfg.LangMap)
	if err != nil {
		return nil, err
	}
	if err := acquireEnv(cfg.ORTLib); err != nil {
		return nil, err
	}
	e := &Engine{ffmpeg: cfg.FFmpeg, langs: langs}
	if e.vad, err = newSession(cfg.VAD, []string{"input", "state", "sr"}, []string{"output", "stateN"}, 1, nil); err != nil {
		releaseEnv()
		return nil, fmt.Errorf("silero vad: %w", err)
	}
	if e.ecapa, err = newSession(cfg.ECAPA, []string{"audio"}, []string{"logits"}, cfg.IntraOpThreads, ecapaOptions); err != nil {
		e.vad.Destroy()
		releaseEnv()
		return nil, fmt.Errorf("ecapa: %w", err)
	}
	return e, nil
}

// ecapaOptions: the input length differs per clip, so ORT's memory-pattern cache (built for
// fixed shapes) only costs RAM: measured peak RSS with 6 concurrent clips 2.1 GB -> 1.0 GB,
// throughput within noise. Spinning threads burn CPU while waiting: 3.3 -> 1.8 CPU-s per clip
// with 6 intra-op threads, throughput not lower. See the benchmark notes in the commit message.
func ecapaOptions(o *ort.SessionOptions) error {
	if err := o.SetMemPattern(false); err != nil {
		return err
	}
	return o.AddSessionConfigEntry("session.intra_op.allow_spinning", "0")
}

func newSession(model string, in, out []string, intraOp int, extra func(*ort.SessionOptions) error) (*ort.DynamicAdvancedSession, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(intraOp); err != nil {
		return nil, err
	}
	if err := opts.SetInterOpNumThreads(1); err != nil {
		return nil, err
	}
	if err := opts.SetLogSeverityLevel(ort.LoggingLevelError); err != nil {
		return nil, err
	}
	if extra != nil {
		if err := extra(opts); err != nil {
			return nil, err
		}
	}
	return ort.NewDynamicAdvancedSession(model, in, out, opts)
}

// Close releases the models. No Detect may be running.
func (e *Engine) Close() error {
	e.vad.Destroy()
	e.ecapa.Destroy()
	releaseEnv()
	return nil
}

// loadLangMap reads {"0": {"iso": "ab", ...}, ...} into a slice indexed by class.
func loadLangMap(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]struct {
		ISO string `json:"iso"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	langs := make([]string, len(m))
	for k, v := range m {
		i, err := strconv.Atoi(k)
		if err != nil || i < 0 || i >= len(langs) || v.ISO == "" {
			return nil, fmt.Errorf("%s: bad entry %q", path, k)
		}
		langs[i] = v.ISO
	}
	return langs, nil
}

// Detect decodes up to the first lid.MaxSeconds of path and identifies the language.
// An error means the clip could not be read; too little speech is Detection.NoSpeech.
func (e *Engine) Detect(ctx context.Context, path string) (lid.Detection, error) {
	pcm, durS, err := e.decode(ctx, path)
	if err != nil {
		return lid.Detection{}, err
	}
	speech, err := e.speech(pcm)
	if err != nil {
		return lid.Detection{}, err
	}
	det := lid.Detection{DurationS: durS, SpeechS: float64(len(speech)) / sampleRate}
	if len(speech) < minSpeechSamples {
		det.NoSpeech = true
		return det, nil
	}
	if err := ctx.Err(); err != nil {
		return lid.Detection{}, err
	}
	if det.Top, err = e.classify(speech); err != nil {
		return lid.Detection{}, err
	}
	return det, nil
}
