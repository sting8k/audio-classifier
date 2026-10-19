package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sting8k/audio-classifier/internal/lid"
)

const maxSamples = lid.MaxSeconds * sampleRate

// ffmpeg flags are SPEC's, except two things:
//   - -loglevel info -nostats instead of -loglevel error: the same process then reports the
//     container's full Duration, so Detect needs no second probe process (~25 ms each).
//   - the input is passed as file:<path> so a name like "a:b.mp3" or "-x.wav" is never read
//     as a protocol or an option.
func decodeArgs(path string) []string {
	return []string{
		"-nostdin", "-hide_banner", "-nostats", "-loglevel", "info",
		"-t", strconv.Itoa(lid.MaxSeconds), "-i", "file:" + path,
		"-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", strconv.Itoa(sampleRate),
		"-f", "f32le", "pipe:1",
	}
}

// decode returns the first lid.MaxSeconds of path as 16 kHz mono float32 and the container's
// full duration (0 if the container does not say and the clip is longer than what was decoded).
func (e *Engine) decode(ctx context.Context, path string) ([]float32, float64, error) {
	cmd := exec.CommandContext(ctx, e.ffmpeg, decodeArgs(path)...)
	hideWindow(cmd)
	var log ffmpegLog
	cmd.Stderr = &log
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	// -t already caps the output; the extra 4 bytes only detect a runaway process.
	raw := make([]byte, 4*maxSamples+4)
	n, _ := io.ReadFull(stdout, raw)
	runaway := n > 4*maxSamples
	if runaway {
		cmd.Process.Kill()
		n = 4 * maxSamples
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if waitErr != nil && !runaway {
		return nil, 0, log.failure(waitErr)
	}
	pcm := make([]float32, n/4)
	if len(pcm) == 0 {
		return nil, 0, errors.New("no audio could be decoded")
	}
	for i := range pcm {
		pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	durS := log.durationS
	if durS == 0 && len(pcm) < maxSamples {
		durS = float64(len(pcm)) / sampleRate // short clip, container gave no duration
	}
	return pcm, durS, nil
}

// Duration returns the clip's full length without decoding it (0 if the container has none).
// It runs ffmpeg with an input and no output: ffmpeg prints the input's description and then
// exits 1 ("At least one output file must be specified"), which is the cheapest way to open
// a file. Opening is ~25 ms either way, so bundling ffprobe would buy nothing.
func (e *Engine) Duration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, e.ffmpeg,
		"-nostdin", "-hide_banner", "-nostats", "-loglevel", "info", "-i", "file:"+path)
	hideWindow(cmd)
	var log ffmpegLog
	cmd.Stderr = &log
	err := cmd.Run()
	if cerr := ctx.Err(); cerr != nil {
		return 0, cerr
	}
	if !log.sawInput {
		if err == nil {
			err = errors.New("unreadable media file")
		}
		return 0, log.failure(err)
	}
	return log.durationS, nil
}

var (
	durationLine = regexp.MustCompile(`^Duration: (\d+):(\d\d):(\d\d(?:\.\d+)?)`)
	logPrefix    = regexp.MustCompile(`^\[[^\]]*\] `) // "[out#0/f32le @ 0x5833d4c13dc0] "
)

// ffmpegLog digests ffmpeg's stderr while it streams: the input's Duration and the last
// message (which is the reason when ffmpeg fails).
type ffmpegLog struct {
	partial   []byte
	sawInput  bool
	durationS float64
	last      string
	noAudio   bool
}

func (l *ffmpegLog) Write(p []byte) (int, error) {
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexAny(l.partial, "\r\n")
		if i < 0 {
			break
		}
		l.line(string(l.partial[:i]))
		l.partial = l.partial[i+1:]
	}
	if len(l.partial) > 4096 { // a runaway line carries nothing we need
		l.partial = l.partial[:0]
	}
	return len(p), nil
}

func (l *ffmpegLog) line(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	// The final stats line and "Conversion failed!" follow the real reason; skip them for l.last.
	if !strings.HasPrefix(s, "size=") && s != "Conversion failed!" {
		l.last = logPrefix.ReplaceAllString(s, "")
	}
	if strings.Contains(s, "matches no streams") {
		l.noAudio = true
	}
	if strings.HasPrefix(s, "Input #0") {
		l.sawInput = true
	}
	if m := durationLine.FindStringSubmatch(s); m != nil && l.durationS == 0 {
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		sec, _ := strconv.ParseFloat(m[3], 64)
		l.durationS = float64(h*3600+mm*60) + sec
	}
}

func (l *ffmpegLog) failure(cause error) error {
	if l.noAudio {
		return errors.New("no audio stream")
	}
	if l.last != "" {
		return errors.New(l.last)
	}
	return fmt.Errorf("ffmpeg: %w", cause)
}
