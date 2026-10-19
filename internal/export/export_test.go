package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

func f(v float64) *float64 { return &v }

// rows has one of each status, Vietnamese and awkward names, and values that
// are already at export precision so a round trip must reproduce them.
func rows() []lid.ClipResult {
	return []lid.ClipResult{
		{Folder: "/Dữ liệu/Đối tác A", File: "tiếng Việt, \"1\".mp3", Path: "/Dữ liệu/Đối tác A/tiếng Việt, \"1\".mp3",
			DurationS: f(292.5), Language: "vi", Confidence: f(0.9712), SecondLanguage: "en", SecondConfidence: f(0.0123), Status: lid.StatusOK},
		{Folder: "/Dữ liệu/Đối tác A", File: "Tom & Jerry <2>.wav", Path: "/Dữ liệu/Đối tác A/Tom & Jerry <2>.wav",
			DurationS: f(60), Language: "th", Confidence: f(0.58), Status: lid.StatusReview},
		{Folder: "/b", File: "mute.m4a", Path: "/b/mute.m4a", DurationS: f(12.25), Status: lid.StatusNoSpeech},
		{Folder: "/b", File: "broken.mp3", Path: "/b/broken.mp3", Status: lid.StatusError, Error: "cannot decode audio"},
	}
}

func export(t *testing.T, req lid.ExportRequest, rs []lid.ClipResult) []byte {
	t.Helper()
	req.Out = filepath.Join(t.TempDir(), "out")
	if err := Write(req, rs, Options{MinConfidence: 0.6, Now: time.Date(2026, 10, 19, 12, 10, 31, 0, time.FixedZone("", 7*3600))}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(req.Out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readCSV(t *testing.T, b []byte, comma rune) [][]string {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\ufeff"))))
	r.Comma = comma
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// "For Excel" (BOM) means BOM and ';'; off means plain UTF-8 and ','.
// readCSV is told the delimiter, so parsing with the wrong one fails the test.
func TestCSVFieldSelectionAndExcelMode(t *testing.T) {
	// Fields arrive in UI order; the file keeps the canonical order.
	req := lid.ExportRequest{Format: "csv", Fields: []string{"status", "file", "confidence"}, BOM: true}
	b := export(t, req, rows())
	if !bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("BOM requested but missing")
	}
	recs := readCSV(t, b, ';')
	if want := []string{"file", "confidence", "status"}; !reflect.DeepEqual(recs[0], want) {
		t.Errorf("header = %v, want %v", recs[0], want)
	}
	if want := []string{"tiếng Việt, \"1\".mp3", "0.9712", "ok"}; !reflect.DeepEqual(recs[1], want) {
		t.Errorf("row 1 = %q, want %q (names intact, comma and quote escaped)", recs[1], want)
	}
	if want := []string{"broken.mp3", "", "error"}; !reflect.DeepEqual(recs[4], want) {
		t.Errorf("row 4 = %q, want %q", recs[4], want)
	}

	req.BOM = false
	b = export(t, req, rows())
	if bytes.HasPrefix(b, []byte{0xEF}) {
		t.Error("BOM present although not requested")
	}
	if recs := readCSV(t, b, ','); !reflect.DeepEqual(recs[1], []string{"tiếng Việt, \"1\".mp3", "0.9712", "ok"}) {
		t.Errorf("without BOM the delimiter must be ',': row 1 = %q", recs[1])
	}
	req.Fields = nil
	if recs := readCSV(t, export(t, req, rows()), ','); !reflect.DeepEqual(recs[0], Fields) {
		t.Errorf("default header = %v, want all of %v", recs[0], Fields)
	}
}

func TestCSVRoundsToExportPrecision(t *testing.T) {
	r := []lid.ClipResult{{Folder: "/a", File: "x.mp3", DurationS: f(292.123456), Confidence: f(0.97123456), Status: lid.StatusOK}}
	recs := readCSV(t, export(t, lid.ExportRequest{Format: "csv", Fields: []string{"duration_s", "confidence"}}, r), ',')
	if want := []string{"292.123", "0.9712"}; !reflect.DeepEqual(recs[1], want) {
		t.Errorf("row = %v, want %v", recs[1], want)
	}
}

// A name that Excel would read as a formula is made text in CSV (leading '),
// and left exactly as it is in JSON.
func TestFormulaLikeNamesAreNeutralisedInCSVOnly(t *testing.T) {
	var in []lid.ClipResult
	for _, n := range []string{"=cmd.mp3", "+1.mp3", "-2.mp3", "@x.mp3", "a=b.mp3"} {
		in = append(in, lid.ClipResult{Folder: "/a", File: n, Path: "/a/" + n, Status: lid.StatusOK})
	}
	recs := readCSV(t, export(t, lid.ExportRequest{Format: "csv", Fields: []string{"file"}}, in), ',')
	want := [][]string{{"file"}, {"'=cmd.mp3"}, {"'+1.mp3"}, {"'-2.mp3"}, {"'@x.mp3"}, {"a=b.mp3"}}
	if !reflect.DeepEqual(recs, want) {
		t.Errorf("csv = %q, want %q (only a leading = + - @ is neutralised)", recs, want)
	}

	var doc struct{ Clips []lid.ClipResult }
	if err := json.Unmarshal(export(t, lid.ExportRequest{Format: "json", Fields: []string{"file"}}, in), &doc); err != nil {
		t.Fatal(err)
	}
	for i, c := range doc.Clips {
		if c.File != in[i].File {
			t.Errorf("json file %d = %q, want %q unchanged", i, c.File, in[i].File)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	in := rows()
	b := export(t, lid.ExportRequest{Format: "json", BOM: true}, in)
	if bytes.HasPrefix(b, []byte{0xEF}) {
		t.Error("BOM written into JSON")
	}
	var doc struct {
		GeneratedAt string `json:"generated_at"`
		Model       string `json:"model"`
		Settings    struct {
			MaxSeconds    int     `json:"max_seconds"`
			MinConfidence float64 `json:"min_confidence"`
		} `json:"settings"`
		Summary lid.Summary      `json:"summary"`
		Clips   []lid.ClipResult `json:"clips"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if doc.GeneratedAt != "2026-10-19T12:10:31+07:00" || doc.Model == "" || doc.Settings.MaxSeconds != lid.MaxSeconds || doc.Settings.MinConfidence != 0.6 {
		t.Errorf("header = %+v", doc)
	}
	if want := (lid.Summary{Total: 4, OK: 1, Review: 1, NoSpeech: 1, Error: 1}); doc.Summary != want {
		t.Errorf("summary = %+v, want %+v", doc.Summary, want)
	}
	want := make([]lid.ClipResult, len(in))
	for i, r := range in {
		r.Path = "" // not an export field
		want[i] = r
	}
	if !reflect.DeepEqual(doc.Clips, want) {
		t.Errorf("clips did not round-trip:\n got %+v\nwant %+v", doc.Clips, want)
	}

	// Unknown values are explicit nulls; "error" appears only where there is one.
	var raw struct{ Clips []map[string]any }
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if v, ok := raw.Clips[3]["language"]; !ok || v != nil {
		t.Errorf("error clip language = %v (present=%v), want null", v, ok)
	}
	if _, ok := raw.Clips[0]["error"]; ok {
		t.Error("ok clip carries an error key")
	}
	if strings.Contains(string(b), `\u0026`) {
		t.Error("'&' in a file name was HTML-escaped")
	}
}

func TestPathsSelectRows(t *testing.T) {
	in := rows()
	recs := readCSV(t, export(t, lid.ExportRequest{Format: "csv", Fields: []string{"file"}, Paths: []string{in[3].Path, in[2].Path, "/nope"}}, in), ',')
	if want := [][]string{{"file"}, {"mute.m4a"}, {"broken.mp3"}}; !reflect.DeepEqual(recs, want) {
		t.Errorf("selected = %v, want %v (listed rows only, in result order)", recs, want)
	}
	if recs := readCSV(t, export(t, lid.ExportRequest{Format: "csv", Fields: []string{"file"}}, in), ','); len(recs) != 5 {
		t.Errorf("no Paths gave %d lines, want header + all 4", len(recs))
	}

	var doc struct{ Summary lid.Summary }
	if err := json.Unmarshal(export(t, lid.ExportRequest{Format: "json", Paths: []string{in[0].Path}}, in), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Summary != (lid.Summary{Total: 1, OK: 1}) {
		t.Errorf("summary of a 1-row export = %+v; it must describe the exported rows", doc.Summary)
	}
}

// A request that cannot be honoured must not touch the file already there.
func TestBadRequestLeavesExistingFileAlone(t *testing.T) {
	out := filepath.Join(t.TempDir(), "results.csv")
	if err := os.WriteFile(out, []byte("previous export"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, req := range map[string]lid.ExportRequest{
		"unknown field":   {Format: "csv", Fields: []string{"file", "path"}},
		"unknown format":  {Format: "xlsx"},
		"no row matches":  {Format: "csv", Paths: []string{"/nope"}},
		"missing out dir": {Format: "csv", Out: filepath.Join(out, "x")},
	} {
		if req.Out == "" {
			req.Out = out
		}
		if err := Write(req, rows(), Options{}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := Write(lid.ExportRequest{Format: "csv", Out: out}, nil, Options{}); err == nil {
		t.Error("exporting zero results: no error")
	}
	if b, _ := os.ReadFile(out); string(b) != "previous export" {
		t.Errorf("existing file changed: %q", b)
	}
	if ents, _ := os.ReadDir(filepath.Dir(out)); len(ents) != 1 {
		t.Errorf("stray files left behind: %v", ents)
	}

	// A destination that cannot be replaced fails loudly and leaves no temp file.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "taken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(lid.ExportRequest{Format: "csv", Out: filepath.Join(dir, "taken")}, rows(), Options{}); err == nil {
		t.Error("export over a directory: no error")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("temp file left behind: %v", ents)
	}
}
