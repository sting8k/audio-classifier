package lid

import (
	"context"
	"errors"
	"hash/fnv"
	"path/filepath"
	"strings"
	"time"
)

// Fake is a deterministic Detector for tests and UI development
// (run the app with LANGID_FAKE=1). Outcome depends only on the file name:
// names containing "err" fail, "silent" have no speech, others get a
// pseudo-random language and confidence.
type Fake struct {
	Delay time.Duration
}

var fakeLangs = []string{"vi", "en", "zh", "th", "pt", "id", "ar"}

func (f Fake) Detect(ctx context.Context, path string) (Detection, error) {
	if f.Delay > 0 {
		select {
		case <-ctx.Done():
			return Detection{}, ctx.Err()
		case <-time.After(f.Delay):
		}
	}
	name := strings.ToLower(filepath.Base(path))
	h := fnv.New32a()
	h.Write([]byte(name))
	n := h.Sum32()
	d := Detection{DurationS: float64(20 + n%300), SpeechS: float64(5 + n%25)}
	switch {
	case strings.Contains(name, "err"):
		return Detection{}, errors.New("cannot decode audio")
	case strings.Contains(name, "silent"):
		d.NoSpeech = true
		d.SpeechS = 0
		return d, nil
	}
	p1 := 0.40 + float64(n%60)/100
	k := uint32(len(fakeLangs))
	i1 := n % k
	i2 := (i1 + 1 + (n/7)%(k-1)) % k // never equal to i1
	d.Top = []LangProb{
		{Lang: fakeLangs[i1], Prob: p1},
		{Lang: fakeLangs[i2], Prob: (1 - p1) / 2},
	}
	return d, nil
}

func (Fake) Close() error { return nil }

func (f Fake) Duration(ctx context.Context, path string) (float64, error) {
	d, err := f.Detect(ctx, path)
	return d.DurationS, err
}
