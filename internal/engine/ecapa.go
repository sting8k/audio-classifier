package engine

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/sting8k/audio-classifier/internal/lid"
	ort "github.com/yalue/onnxruntime_go"
)

// classify runs one ECAPA pass over speech and returns the two most probable languages.
func (e *Engine) classify(speech []float32) ([]lid.LangProb, error) {
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(speech))), speech)
	if err != nil {
		return nil, err
	}
	defer in.Destroy()
	out, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(len(e.langs))))
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := e.ecapa.Run([]ort.Value{in}, []ort.Value{out}); err != nil {
		return nil, err
	}
	return top2(softmax(out.GetData()), e.langs)
}

func softmax(logits []float32) []float64 {
	maxv := logits[0]
	for _, v := range logits {
		maxv = max(maxv, v)
	}
	p := make([]float64, len(logits))
	sum := 0.0
	for i, v := range logits {
		p[i] = math.Exp(float64(v - maxv))
		sum += p[i]
	}
	for i := range p {
		p[i] /= sum
	}
	return p
}

func top2(p []float64, langs []string) ([]lid.LangProb, error) {
	if len(p) != len(langs) {
		return nil, fmt.Errorf("model has %d classes, lang_map has %d", len(p), len(langs))
	}
	if math.IsNaN(p[0]) { // NaN would make the whole JSON export fail
		return nil, errors.New("model returned NaN")
	}
	idx := make([]int, len(p))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return p[idx[a]] > p[idx[b]] })
	return []lid.LangProb{{Lang: langs[idx[0]], Prob: p[idx[0]]}, {Lang: langs[idx[1]], Prob: p[idx[1]]}}, nil
}
