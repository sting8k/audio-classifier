package engine

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/sting8k/audio-classifier/internal/lid"
	ort "github.com/yalue/onnxruntime_go"
	"gonum.org/v1/gonum/dsp/fourier"
)

// Ear's front end (ear_meta.json "frontend"): Whisper-style log-mel over one 30 s window.
const (
	melFFT    = 400
	melHop    = 160
	melFrames = 3000
	melWindow = melFrames * melHop // 480000 samples = 30 s
)

// classify runs one Ear pass over speech (zero-padded or cut to 30 s) and returns the two
// most probable languages.
func (e *Engine) classify(speech []float32) ([]lid.LangProb, error) {
	mel := e.logMel(speech)
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(e.mel)), melFrames), mel)
	if err != nil {
		return nil, err
	}
	defer in.Destroy()
	out, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(len(e.langs))))
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := e.model.Run([]ort.Value{in}, []ort.Value{out}); err != nil {
		return nil, err
	}
	return top2(softmax(out.GetData()), e.langs)
}

// logMel returns the model input, mel-major ([mel][frame]): power spectrum of Hann-windowed
// 400-sample frames every 160 samples (centred, reflect-padded), mel filterbank, log10,
// floor at peak-8, then (x+4)/4. Same steps as the reference Python front end.
func (e *Engine) logMel(speech []float32) []float32 {
	x := make([]float64, melWindow+melFFT)
	const pad = melFFT / 2
	for i := 0; i < melWindow && i < len(speech); i++ {
		x[pad+i] = float64(speech[i])
	}
	for i := 1; i <= pad; i++ { // reflect, edge sample not repeated
		x[pad-i] = x[pad+i]
		x[pad+melWindow-1+i] = x[pad+melWindow-1-i]
	}
	fft := fourier.NewFFT(melFFT) // not safe for concurrent use: one per call
	frame := make([]float64, melFFT)
	coef := make([]complex128, melFFT/2+1)
	nBins := len(coef)
	power := make([]float64, melFrames*nBins) // [frame][bin]; the 3001st frame is dropped
	for t := range melFrames {
		for i := range frame {
			frame[i] = x[t*melHop+i] * e.hann[i]
		}
		fft.Coefficients(coef, frame)
		for k, c := range coef {
			power[t*nBins+k] = real(c)*real(c) + imag(c)*imag(c)
		}
	}
	out := make([]float32, len(e.mel)*melFrames)
	peak := math.Inf(-1)
	logs := make([]float64, len(out))
	for m, filt := range e.mel {
		for t := range melFrames {
			p := power[t*nBins : (t+1)*nBins]
			s := 0.0
			for k, w := range filt {
				s += w * p[k]
			}
			v := math.Log10(max(s, 1e-10))
			logs[m*melFrames+t] = v
			peak = max(peak, v)
		}
	}
	for i, v := range logs {
		out[i] = float32((max(v, peak-8) + 4) / 4)
	}
	return out
}

// loadMelFilters reads mel_filters.f32: little-endian uint32 rows, uint32 cols, then rows*cols
// float32 (rows = mel bands, cols = FFT bins).
func loadMelFilters(path string) ([][]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("%s: too short", path)
	}
	rows, cols := int(binary.LittleEndian.Uint32(raw)), int(binary.LittleEndian.Uint32(raw[4:]))
	if cols != melFFT/2+1 || len(raw) != 8+4*rows*cols {
		return nil, fmt.Errorf("%s: %dx%d filterbank does not fit a %d-point FFT", path, rows, cols, melFFT)
	}
	fb := make([][]float64, rows)
	for r := range fb {
		fb[r] = make([]float64, cols)
		for c := range fb[r] {
			fb[r][c] = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[8+4*(r*cols+c):])))
		}
	}
	return fb, nil
}

// hannPeriodic is the periodic Hann window (torch.hann_window default).
func hannPeriodic(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}

// loadLanguages reads languages.json: the ISO code of each output class, in class order.
func loadLanguages(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var langs []string
	if err := json.Unmarshal(raw, &langs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, l := range langs {
		if l == "" {
			return nil, fmt.Errorf("%s: empty code at %d", path, i)
		}
	}
	return langs, nil
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
		return nil, fmt.Errorf("model has %d classes, languages.json has %d", len(p), len(langs))
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
