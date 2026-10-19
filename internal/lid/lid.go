// Package lid holds the contract shared by the engine (audio -> language),
// the batch runner (folders -> results) and the Wails bindings (results -> UI).
// Changing a type here changes the seam between scopes: lead-owned.
package lid

import "context"

// MaxSeconds is how much of each clip is analysed (SPEC D-1).
const MaxSeconds = 30

// LangProb is one language class and its softmax probability.
type LangProb struct {
	Lang string  `json:"lang"` // ISO 639-1 code from the model's lang_map (e.g. "pt")
	Prob float64 `json:"prob"`
}

// Detection is the raw engine output for one clip. It has no status:
// thresholds are user settings applied by the batch runner.
type Detection struct {
	DurationS float64 // full clip duration in seconds; 0 if unknown
	SpeechS   float64 // seconds of speech VAD kept from the first MaxSeconds
	NoSpeech  bool    // too little speech to classify; Top is empty then
	Top       []LangProb
	// Top holds the best two classes, highest first (len 2 unless NoSpeech).
}

// Detector turns one media file into a Detection.
// A returned error means the clip could not be read/decoded (status "error").
// Implementations must be safe for concurrent use by several goroutines.
type Detector interface {
	Detect(ctx context.Context, path string) (Detection, error)
	Close() error
}

// Prober reads a clip's full duration without decoding it (screen-1 totals,
// ClipResult.DurationS). Implemented by the engine; safe for concurrent use.
type Prober interface {
	Duration(ctx context.Context, path string) (float64, error)
}

// Status of one clip result (SPEC "Statuses").
type Status string

const (
	StatusOK       Status = "ok"
	StatusReview   Status = "review"
	StatusNoSpeech Status = "no_speech"
	StatusError    Status = "error"
)

// ClipResult is one row of the results table and of the export.
// JSON tags are the export field names (SPEC "Export").
type ClipResult struct {
	Folder           string   `json:"folder"`
	File             string   `json:"file"`
	Path             string   `json:"path"`
	DurationS        *float64 `json:"duration_s"`
	SpeechS          *float64 `json:"speech_s"`
	Language         string   `json:"language"`
	Confidence       *float64 `json:"confidence"`
	SecondLanguage   string   `json:"second_language"`
	SecondConfidence *float64 `json:"second_confidence"`
	Status           Status   `json:"status"`
	Error            string   `json:"error,omitempty"`
}
