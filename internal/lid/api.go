package lid

// Types below cross the Go <-> frontend boundary through the Wails bindings
// (generated into frontend/wailsjs/go/models.ts). Lead-owned contract.

// Settings chosen on screen 1 (SPEC "Inputs", D-7).
type Settings struct {
	Extensions    []string `json:"extensions"`     // lower-case, no dot: "mp3", "mp4"...
	Recursive     bool     `json:"recursive"`      // scan sub-folders
	MinConfidence float64  `json:"min_confidence"` // below -> StatusReview; default 0.60
	Workers       int      `json:"workers"`        // 0 = auto
}

// DefaultExtensions are the formats offered on screen 1.
var DefaultExtensions = []string{"mp3", "wav", "m4a", "aac", "flac", "ogg", "mp4", "mov"}

// DefaultMinConfidence is a guess, not measured (SPEC D-7).
const DefaultMinConfidence = 0.60

// FolderSummary is one row of the folder table on screen 1.
type FolderSummary struct {
	Path           string         `json:"path"`
	Name           string         `json:"name"`
	Clips          int            `json:"clips"`            // files matching Settings.Extensions
	Skipped        map[string]int `json:"skipped"`          // other files by extension ("txt": 2)
	TotalDurationS *float64       `json:"total_duration_s"` // nil until known
	Error          string         `json:"error,omitempty"`  // unreadable folder
}

// Run states.
const (
	StateIdle      = "idle"
	StateRunning   = "running"
	StatePaused    = "paused"
	StateDone      = "done"
	StateCancelled = "cancelled"
)

// Summary counts per status.
type Summary struct {
	Total    int `json:"total"`
	OK       int `json:"ok"`
	Review   int `json:"review"`
	NoSpeech int `json:"no_speech"`
	Error    int `json:"error"`
}

// FolderProgress is one row of "Theo thư mục" on screen 2.
type FolderProgress struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Progress is the snapshot shown on screen 2. Emitted as event EventProgress
// (throttled) and returned by App.Progress().
type Progress struct {
	State    string           `json:"state"`
	Done     int              `json:"done"`
	Total    int              `json:"total"`
	ElapsedS float64          `json:"elapsed_s"`
	EtaS     *float64         `json:"eta_s"`
	Current  string           `json:"current"` // path being processed, "" if none
	Folders  []FolderProgress `json:"folders"`
	Recent   []ClipResult     `json:"recent"` // newest first, at most 5
	Summary  Summary          `json:"summary"`
}

// EventProgress is the Wails event name carrying a Progress payload.
const EventProgress = "progress"

// ExportRequest is what screen 4 sends. The frontend resolves the scope
// (all / current filter / selected folders) into Paths; empty Paths = all results.
type ExportRequest struct {
	Format string   `json:"format"` // "json" | "csv"
	Paths  []string `json:"paths"`
	Fields []string `json:"fields"` // ClipResult JSON tag names to include
	BOM    bool     `json:"bom"`    // CSV only: UTF-8 BOM for Excel
	Out    string   `json:"out"`    // destination file path
}
