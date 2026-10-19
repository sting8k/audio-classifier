// Package export writes clip results as JSON or CSV (SPEC "Export").
package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sting8k/audio-classifier/internal/lid"
)

// Model is the model name written into the JSON header.
const Model = "speechbrain/lang-id-voxlingua107-ecapa"

// Fields are the exportable columns, in output order (SPEC "Export").
var Fields = []string{"folder", "file", "duration_s", "language", "confidence",
	"second_language", "second_confidence", "status", "error"}

// Options carries what is not in the request or the rows.
type Options struct {
	MinConfidence float64   // the run's threshold, recorded in the JSON header
	Now           time.Time // generated_at; zero = time.Now()
}

type column struct {
	name     string
	value    func(lid.ClipResult) any // nil = no value (JSON null, CSV empty)
	omitNull bool                     // JSON: leave the key out instead of null
}

var columns = []column{
	{name: "folder", value: func(c lid.ClipResult) any { return c.Folder }},
	{name: "file", value: func(c lid.ClipResult) any { return c.File }},
	{name: "duration_s", value: func(c lid.ClipResult) any { return num(c.DurationS, 1e3) }},
	{name: "language", value: func(c lid.ClipResult) any { return str(c.Language) }},
	{name: "confidence", value: func(c lid.ClipResult) any { return num(c.Confidence, 1e4) }},
	{name: "second_language", value: func(c lid.ClipResult) any { return str(c.SecondLanguage) }},
	{name: "second_confidence", value: func(c lid.ClipResult) any { return num(c.SecondConfidence, 1e4) }},
	{name: "status", value: func(c lid.ClipResult) any { return string(c.Status) }},
	{name: "error", value: func(c lid.ClipResult) any { return str(c.Error) }, omitNull: true},
}

// num rounds to 1/scale: 3 decimals for seconds, 4 for probabilities.
func num(p *float64, scale float64) any {
	if p == nil {
		return nil
	}
	return math.Round(*p*scale) / scale
}

func str(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Write exports the rows chosen by req.Paths (empty = all) to req.Out.
// The file is replaced atomically: a failed export leaves the old one intact.
func Write(req lid.ExportRequest, rows []lid.ClipResult, o Options) error {
	if req.Format != "json" && req.Format != "csv" {
		return fmt.Errorf("unsupported format %q", req.Format)
	}
	cols, err := selectColumns(req.Fields)
	if err != nil {
		return err
	}
	rows = selectRows(rows, req.Paths)
	if len(rows) == 0 {
		return errors.New("no clips to export")
	}
	if req.Out == "" {
		return errors.New("no output file chosen")
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	var buf bytes.Buffer
	if req.Format == "json" {
		err = encodeJSON(&buf, cols, rows, o)
	} else {
		err = encodeCSV(&buf, cols, rows, req.BOM)
	}
	if err != nil {
		return err
	}
	return replaceFile(req.Out, buf.Bytes())
}

// selectColumns keeps the canonical column order whatever order the UI sent.
func selectColumns(want []string) ([]column, error) {
	if len(want) == 0 {
		return columns, nil
	}
	left := make(map[string]bool, len(want))
	for _, f := range want {
		left[f] = true
	}
	var out []column
	for _, c := range columns {
		if left[c.name] {
			out = append(out, c)
			delete(left, c.name)
		}
	}
	for f := range left {
		return nil, fmt.Errorf("unknown field %q", f)
	}
	return out, nil
}

func selectRows(rows []lid.ClipResult, paths []string) []lid.ClipResult {
	if len(paths) == 0 {
		return rows
	}
	keep := make(map[string]bool, len(paths))
	for _, p := range paths {
		keep[p] = true
	}
	var out []lid.ClipResult
	for _, r := range rows {
		if keep[r.Path] {
			out = append(out, r)
		}
	}
	return out
}

func encodeCSV(buf *bytes.Buffer, cols []column, rows []lid.ClipResult, bom bool) error {
	if bom {
		buf.WriteString("\ufeff") // EF BB BF: Excel on Windows needs it to read UTF-8
	}
	w := csv.NewWriter(buf)
	w.UseCRLF = true
	rec := make([]string, len(cols))
	for i, c := range cols {
		rec[i] = c.name
	}
	if err := w.Write(rec); err != nil {
		return err
	}
	for _, r := range rows {
		for i, c := range cols {
			switch v := c.value(r).(type) {
			case nil:
				rec[i] = ""
			case string:
				rec[i] = v
			case float64:
				rec[i] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// jsonRow is a clip with its keys in column order (a map would sort them).
type jsonRow struct {
	cols []column
	c    lid.ClipResult
}

func (r jsonRow) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for _, col := range r.cols {
		v := col.value(r.c)
		if v == nil && col.omitNull {
			continue
		}
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		k, err := marshal(col.name)
		if err != nil {
			return nil, err
		}
		val, err := marshal(v)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshal is json.Marshal without HTML escaping, so "Tom & Jerry.mp3" stays readable.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

func encodeJSON(buf *bytes.Buffer, cols []column, rows []lid.ClipResult, o Options) error {
	type settings struct {
		MaxSeconds    int     `json:"max_seconds"`
		MinConfidence float64 `json:"min_confidence"`
	}
	doc := struct {
		GeneratedAt string      `json:"generated_at"`
		Model       string      `json:"model"`
		Settings    settings    `json:"settings"`
		Summary     lid.Summary `json:"summary"`
		Clips       []jsonRow   `json:"clips"`
	}{
		GeneratedAt: o.Now.Format(time.RFC3339),
		Model:       Model,
		Settings:    settings{MaxSeconds: lid.MaxSeconds, MinConfidence: o.MinConfidence},
		Clips:       make([]jsonRow, len(rows)),
	}
	for i, r := range rows {
		doc.Clips[i] = jsonRow{cols, r}
		doc.Summary.Total++
		switch r.Status {
		case lid.StatusOK:
			doc.Summary.OK++
		case lid.StatusReview:
			doc.Summary.Review++
		case lid.StatusNoSpeech:
			doc.Summary.NoSpeech++
		case lid.StatusError:
			doc.Summary.Error++
		}
	}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// replaceFile writes data next to path and renames it over path.
func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".langid-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // nothing left to remove after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	_ = tmp.Chmod(0o644) // CreateTemp makes it 0600; best effort, not worth failing the export
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
