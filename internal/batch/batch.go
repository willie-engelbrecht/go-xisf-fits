// Package batch walks a directory tree and converts each XISF file to FITS.
package batch

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"go-xisf-fits/internal/fits"
	"go-xisf-fits/internal/xisf"
)

// ErrNoFiles is returned when the input directory contains no XISF files.
// The log file is still written.
var ErrNoFiles = errors.New("no XISF files found")

// Summary is the result of one batch.
type Summary struct {
	LogPath   string
	Succeeded int
	Failed    int
}

// DefaultCores is half the machine's logical CPUs, and at least one.
func DefaultCores() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		return 1
	}
	return n
}

// ParseCores accepts a worker count from 1 through the machine's logical CPUs.
func ParseCores(n int) (int, error) {
	max := runtime.NumCPU()
	if n < 1 || n > max {
		return 0, fmt.Errorf("cores must be from 1 to %d", max)
	}
	return n, nil
}

// Run converts every .xisf file under input into a mirrored path under output.
// cores is how many files are converted at once, one file per worker.
// Each result line is written to live and to a timestamped log in output.
// input must already exist. output is created when it is missing.
func Run(input, output string, live io.Writer, opt fits.Options, cores int, progress func(done, total int)) (Summary, error) {
	if err := opt.Validate(); err != nil {
		return Summary{}, err
	}
	cores, err := ParseCores(cores)
	if err != nil {
		return Summary{}, err
	}
	absIn, absOut, err := Check(input, output)
	if err != nil {
		return Summary{}, err
	}
	logPath, logFile, err := createLog(absOut, time.Now())
	if err != nil {
		return Summary{}, err
	}
	defer logFile.Close()

	lw := &lineWriter{log: logFile, live: live}
	files, err := findXISF(absIn, absOut)
	if err != nil {
		lw.line("FAIL %s: %s", absIn, oneLine(err))
		lw.summary(0, 1, logPath)
		return Summary{LogPath: logPath, Failed: 1}, err
	}
	lw.line("compression: %s", opt.Describe())
	if len(files) == 0 {
		lw.line("FAIL %s: no XISF files found", absIn)
		lw.summary(0, 0, logPath)
		return Summary{LogPath: logPath}, ErrNoFiles
	}
	if progress != nil {
		progress(0, len(files))
	}

	prev := runtime.GOMAXPROCS(cores)
	defer runtime.GOMAXPROCS(prev)

	workers := cores
	if workers > len(files) {
		workers = len(files)
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var summary Summary
	summary.LogPath = logPath
	done := 0
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for inPath := range jobs {
				outPath, size, convErr := convertOne(absIn, absOut, inPath, opt)
				mu.Lock()
				if convErr != nil {
					summary.Failed++
					lw.line("FAIL %s: %s", inPath, oneLine(convErr))
				} else {
					summary.Succeeded++
					lw.line("OK %s -> %s (%s)", inPath, outPath, formatSize(size))
				}
				done++
				if progress != nil {
					progress(done, len(files))
				}
				mu.Unlock()
			}
		}()
	}
	for _, inPath := range files {
		jobs <- inPath
	}
	close(jobs)
	wg.Wait()
	lw.summary(summary.Succeeded, summary.Failed, logPath)
	return summary, nil
}

func convertOne(absIn, absOut, inPath string, opt fits.Options) (string, int64, error) {
	outPath, err := outputPath(absIn, absOut, inPath)
	if err != nil {
		return "", 0, err
	}
	img, err := xisf.Read(inPath)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", 0, err
	}
	if err := fits.Write(outPath, img, opt); err != nil {
		return "", 0, err
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return "", 0, err
	}
	return outPath, info.Size(), nil
}

// formatSize reports n in 1024-based units with one decimal, such as 62.3Mb.
func formatSize(n int64) string {
	if n < 1024 {
		if n == 1 {
			return "1 byte"
		}
		return fmt.Sprintf("%d bytes", n)
	}
	units := []string{"Kb", "Mb", "Gb", "Tb"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

// Check confirms input is an existing directory and creates output if needed.
func Check(input, output string) (string, string, error) {
	if strings.TrimSpace(input) == "" || strings.TrimSpace(output) == "" {
		return "", "", fmt.Errorf("--input and --output are required")
	}
	absIn, err := filepath.Abs(input)
	if err != nil {
		return "", "", fmt.Errorf("input directory: %w", err)
	}
	info, err := os.Stat(absIn)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("input directory %q is not an existing directory", input)
	}
	absOut, err := filepath.Abs(output)
	if err != nil {
		return "", "", fmt.Errorf("output directory: %w", err)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}
	return absIn, absOut, nil
}

func createLog(dir string, now time.Time) (string, *os.File, error) {
	stamp := now.Format("2006_01_02-15-04-05")
	for i := 0; i < 1000; i++ {
		name := stamp + "_output.txt"
		if i > 0 {
			name = fmt.Sprintf("%s_%d_output.txt", stamp, i+1)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create log file: %w", err)
		}
		return path, f, nil
	}
	return "", nil, fmt.Errorf("create log file: too many logs in the same second")
}

func findXISF(input, output string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the output tree when it sits inside the input tree so a
			// previous run is not walked again. When both paths are the same
			// directory, keep walking.
			if !samePath(output, input) && !samePath(path, input) && sameOrInside(path, output) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".xisf") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func outputPath(input, output, inPath string) (string, error) {
	rel, err := filepath.Rel(input, inPath)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(rel)
	base := rel[:len(rel)-len(ext)]
	return filepath.Join(output, base+".fits"), nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func sameOrInside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

type lineWriter struct {
	log  *os.File
	live io.Writer
}

func (w *lineWriter) line(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, _ = w.log.WriteString(s)
	_ = w.log.Sync()
	if w.live != nil {
		_, _ = w.live.Write([]byte(s))
	}
}

func (w *lineWriter) summary(ok, failed int, logPath string) {
	w.line("%d succeeded, %d failed", ok, failed)
	w.line("log file: %s", logPath)
}

func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
