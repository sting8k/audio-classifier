package engine

import (
	ort "github.com/yalue/onnxruntime_go"
)

// Silero VAD v6 streaming model at 16 kHz: 512-sample windows, each fed with the previous
// window's last 64 samples as context, LSTM state carried between windows.
const (
	vadWindow  = 512
	vadContext = 64
)

// Parameters of the speech/silence state machine. Values are silero-vad's get_speech_timestamps
// defaults (threshold 0.5, neg_threshold = threshold-0.15, min_speech 250 ms, min_silence 100 ms,
// pad 30 ms); max_speech_duration is infinite there, so its branch is not ported.
const (
	vadThreshold    = 0.5
	vadNegThreshold = vadThreshold - 0.15
	vadMinSpeech    = sampleRate * 250 / 1000
	vadMinSilence   = sampleRate * 100 / 1000
	vadPad          = sampleRate * 30 / 1000
)

// minSpeechSamples: below this much kept speech Detect reports NoSpeech.
const minSpeechSamples = 3 * sampleRate

// speech returns the VAD-kept parts of pcm, concatenated.
func (e *Engine) speech(pcm []float32) ([]float32, error) {
	probs, err := e.vadProbs(pcm)
	if err != nil {
		return nil, err
	}
	var out []float32
	for _, s := range speechSpans(probs, len(pcm)) {
		out = append(out, pcm[s.start:s.end]...)
	}
	return out, nil
}

// vadProbs returns the speech probability of every 512-sample window of pcm.
func (e *Engine) vadProbs(pcm []float32) ([]float32, error) {
	input := make([]float32, vadContext+vadWindow) // context part starts as zeros
	state := make([]float32, 2*128)
	inT, err := ort.NewTensor(ort.NewShape(1, int64(len(input))), input)
	if err != nil {
		return nil, err
	}
	defer inT.Destroy()
	stateT, err := ort.NewTensor(ort.NewShape(2, 1, 128), state)
	if err != nil {
		return nil, err
	}
	defer stateT.Destroy()
	srT, err := ort.NewScalar(int64(sampleRate))
	if err != nil {
		return nil, err
	}
	defer srT.Destroy()
	outT, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1))
	if err != nil {
		return nil, err
	}
	defer outT.Destroy()
	stateOutT, err := ort.NewEmptyTensor[float32](ort.NewShape(2, 1, 128))
	if err != nil {
		return nil, err
	}
	defer stateOutT.Destroy()

	inputs := []ort.Value{inT, stateT, srT}
	outputs := []ort.Value{outT, stateOutT}
	probs := make([]float32, (len(pcm)+vadWindow-1)/vadWindow)
	for w := range probs {
		copy(input[:vadContext], input[vadWindow:]) // previous window's tail becomes the context
		n := copy(input[vadContext:], pcm[w*vadWindow:])
		clear(input[vadContext+n:]) // last window is zero-padded
		if err := e.vad.Run(inputs, outputs); err != nil {
			return nil, err
		}
		probs[w] = outT.GetData()[0]
		copy(state, stateOutT.GetData())
	}
	return probs, nil
}

type span struct{ start, end int }

// speechSpans is silero-vad's get_speech_timestamps_from_probs (v6.2.3) with
// max_speech_duration_s = inf: hysteresis between threshold and neg_threshold, spans shorter
// than min_speech dropped, silences shorter than min_silence bridged, then each span padded
// by 30 ms (neighbours split the gap between them when it is shorter than 2 pads).
func speechSpans(probs []float32, n int) []span {
	var spans []span
	var cur span
	triggered := false
	tempEnd := 0 // start of the silence that may end the current span; 0 = none
	for i, p := range probs {
		pos := vadWindow * i
		if p >= vadThreshold && tempEnd != 0 {
			tempEnd = 0
		}
		if p >= vadThreshold && !triggered {
			triggered = true
			cur.start = pos
			continue
		}
		if p < vadNegThreshold && triggered {
			if tempEnd == 0 {
				tempEnd = pos
			}
			if pos-tempEnd < vadMinSilence {
				continue
			}
			cur.end = tempEnd
			if cur.end-cur.start > vadMinSpeech {
				spans = append(spans, cur)
			}
			cur, tempEnd, triggered = span{}, 0, false
		}
	}
	if triggered && n-cur.start > vadMinSpeech {
		cur.end = n
		spans = append(spans, cur)
	}
	for i := range spans {
		if i == 0 {
			spans[0].start = max(0, spans[0].start-vadPad)
		}
		if i == len(spans)-1 {
			spans[i].end = min(n, spans[i].end+vadPad)
			continue
		}
		gap := spans[i+1].start - spans[i].end
		if gap < 2*vadPad {
			spans[i].end += gap / 2
			spans[i+1].start = max(0, spans[i+1].start-gap/2)
		} else {
			spans[i].end = min(n, spans[i].end+vadPad)
			spans[i+1].start = max(0, spans[i+1].start-vadPad)
		}
	}
	return spans
}
