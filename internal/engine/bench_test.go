package engine

// Throughput / threading measurements (not an invariant test; skipped unless ENGINE_BENCH is set).
//
//	ENGINE_BENCH=1 ENGINE_BENCH_CLIPS=$HOME/scratch/lid/data go test ./internal/engine -run TestBench -v -timeout 3h
//
// Optional: ENGINE_BENCH_COPIES (default 10 -> 30 clips), ENGINE_BENCH_REPS (3),
// ENGINE_BENCH_WORKERS ("1,2,3,4,6"), ENGINE_BENCH_THREADS ("1,2,3,6"), ENGINE_BENCH_MODES ("shared,perworker").
//
// "shared": one Engine (one ECAPA session, one VAD session) used by W goroutines.
// "perworker": W Engines, goroutine i owns Engine i (its own sessions, each with T threads).
// Linux only: peak RSS is read from /proc after resetting the high-water mark.

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func envInts(name, def string) []int {
	var out []int
	for _, f := range strings.Split(envOr(name, def), ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// benchCorpus copies the clips in ENGINE_BENCH_CLIPS under distinct names.
func benchCorpus(t *testing.T) []string {
	t.Helper()
	src, _ := filepath.Glob(filepath.Join(os.Getenv("ENGINE_BENCH_CLIPS"), "*.mp4"))
	if len(src) == 0 {
		t.Fatal("ENGINE_BENCH_CLIPS has no *.mp4")
	}
	dir := t.TempDir()
	var files []string
	for i := range envInts("ENGINE_BENCH_COPIES", "10")[0] {
		for j, s := range src {
			b, err := os.ReadFile(s)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, fmt.Sprintf("clip_%02d_%d.mp4", i, j))
			os.WriteFile(p, b, 0o644)
			files = append(files, p)
		}
	}
	return files
}

func resetPeakRSS() { os.WriteFile("/proc/self/clear_refs", []byte("5"), 0o200) }
func peakRSSMiB() float64 {
	b, _ := os.ReadFile("/proc/self/status")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "VmHWM:") {
			kb, _ := strconv.Atoi(strings.Fields(l)[1])
			return float64(kb) / 1024
		}
	}
	return 0
}

type cellResult struct {
	wall     time.Duration
	rssMiB   float64
	cpuClip  float64 // CPU seconds per clip, this process + ffmpeg children
	foreignC float64 // CPU of everything else on the box during the run, in cores
}

// runCell processes all files with w goroutines.
func runCell(t *testing.T, mode string, w, threads int, files []string) cellResult {
	cfg := assetsConfig(t)
	cfg.IntraOpThreads = threads
	n := 1
	if mode == "perworker" {
		n = w
	}
	resetPeakRSS()
	engines := make([]*Engine, n)
	for i := range engines {
		e, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		engines[i] = e
	}
	for _, e := range engines { // warm-up, untimed: first Run on each session allocates
		if _, err := e.Detect(context.Background(), files[0]); err != nil {
			t.Fatal(err)
		}
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	busy0, cpu0 := busyJiffies(), myCPU()
	start := time.Now()
	for i := range w {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := engines[i%n]
			for f := range jobs {
				if _, err := e.Detect(context.Background(), f); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	wall := time.Since(start)
	mine := myCPU() - cpu0
	return cellResult{wall, peakRSSMiB(), mine / float64(len(files)), (busyJiffies() - busy0 - mine) / wall.Seconds()}
}

func TestBenchGrid(t *testing.T) {
	if os.Getenv("ENGINE_BENCH") == "" {
		t.Skip("ENGINE_BENCH not set")
	}
	files := benchCorpus(t)
	reps := envInts("ENGINE_BENCH_REPS", "3")[0]
	type cell struct {
		mode       string
		w, threads int
	}
	var cells []cell
	for _, mode := range strings.Split(envOr("ENGINE_BENCH_MODES", "shared,perworker"), ",") {
		for _, w := range envInts("ENGINE_BENCH_WORKERS", "1,2,3,4,6") {
			for _, th := range envInts("ENGINE_BENCH_THREADS", "1,2,3,6") {
				cells = append(cells, cell{mode, w, th})
			}
		}
	}
	rate := map[cell][]float64{}
	rss := map[cell][]float64{}
	rng := rand.New(rand.NewPCG(7, 7))
	for range reps { // rounds in random cell order, so slow drift hits every cell alike
		rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
		for _, c := range cells {
			r := runCell(t, c.mode, c.w, c.threads, files)
			rate[c] = append(rate[c], float64(len(files))/r.wall.Seconds())
			rss[c] = append(rss[c], r.rssMiB)
			fmt.Printf("RESULT mode=%s w=%d t=%d clips_s=%.3f wall_s=%.2f rss_mib=%.0f cpu_s_clip=%.3f foreign_cores=%.2f\n",
				c.mode, c.w, c.threads, rate[c][len(rate[c])-1], r.wall.Seconds(), r.rssMiB, r.cpuClip, r.foreignC)
		}
	}
	fmt.Printf("\n%d clips, %d reps, median clips/s [min-max] -> s per 100 clips, peak RSS MiB (median)\n", len(files), reps)
	sort.Slice(cells, func(i, j int) bool {
		a, b := cells[i], cells[j]
		if a.mode != b.mode {
			return a.mode < b.mode
		}
		if a.w != b.w {
			return a.w < b.w
		}
		return a.threads < b.threads
	})
	for _, c := range cells {
		r := append([]float64(nil), rate[c]...)
		sort.Float64s(r)
		m := append([]float64(nil), rss[c]...)
		sort.Float64s(m)
		med := r[len(r)/2]
		fmt.Printf("%-9s w=%d t=%d  %5.2f [%.2f-%.2f] clips/s  %5.1f s/100  spread %4.1f%%  rss %4.0f\n",
			c.mode, c.w, c.threads, med, r[0], r[len(r)-1], 100/med, 100*(r[len(r)-1]-r[0])/med, m[len(m)/2])
	}
}

// busyJiffies is the all-CPU busy time (user+nice+system+irq+softirq+steal) from /proc/stat.
func busyJiffies() float64 {
	b, _ := os.ReadFile("/proc/stat")
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])[1:]
	var busy float64
	for i, s := range f {
		v, _ := strconv.ParseFloat(s, 64)
		if i != 3 && i != 4 { // idle, iowait
			busy += v
		}
	}
	return busy / 100 // USER_HZ
}

// myCPU is this process's plus its waited-for children's (ffmpeg) CPU seconds.
func myCPU() float64 {
	var a, c syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &a)
	syscall.Getrusage(syscall.RUSAGE_CHILDREN, &c)
	tv := func(t syscall.Timeval) float64 { return float64(t.Sec) + float64(t.Usec)/1e6 }
	return tv(a.Utime) + tv(a.Stime) + tv(c.Utime) + tv(c.Stime)
}
