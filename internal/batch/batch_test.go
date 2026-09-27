package batch

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"xisfits/internal/fits"
)

func TestRunMirrorsSubfolders(t *testing.T) {
	root := repoRoot(t)
	src := filepath.Join(root, "tests", "images", "xisf-image-gray-256x256-8bits.xisf")
	bad := filepath.Join(root, "tests", "images", "xisf-image-rgb-256x256-8bits-compressed.xisf")
	in := t.TempDir()
	out := t.TempDir()
	copyFile(t, src, filepath.Join(in, "night", "sub", "frame.xisf"))
	copyFile(t, src, filepath.Join(in, "frame2.XISF"))
	copyFile(t, bad, filepath.Join(in, "bad", "embedded.xisf"))

	var live bytes.Buffer
	summary, err := Run(in, out, &live, fits.Options{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Succeeded != 2 || summary.Failed != 1 {
		t.Fatalf("succeeded %d failed %d\n%s", summary.Succeeded, summary.Failed, live.String())
	}
	for _, rel := range []string{
		filepath.Join("night", "sub", "frame.fits"),
		"frame2.fits",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "bad")); !os.IsNotExist(err) {
		t.Fatal("failed conversion created an output folder")
	}
	text := live.String()
	if !strings.Contains(text, "OK ") || !strings.Contains(text, " (") || !strings.Contains(text, "FAIL ") || !strings.Contains(text, "embedded") {
		t.Fatalf("log missing result lines:\n%s", text)
	}
	if !strings.Contains(text, "2 succeeded, 1 failed") {
		t.Fatalf("summary missing:\n%s", text)
	}
	logged, err := os.ReadFile(summary.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logged) != text {
		t.Fatal("log file does not match printed output")
	}
	if !strings.HasSuffix(filepath.Base(summary.LogPath), "_output.txt") {
		t.Fatalf("log name %s", summary.LogPath)
	}
}

func TestRunRequiresInputDirectory(t *testing.T) {
	_, err := Run(filepath.Join(t.TempDir(), "missing"), t.TempDir(), ioDiscard{}, fits.Options{}, 1, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunNoFiles(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	var live bytes.Buffer
	summary, err := Run(in, out, &live, fits.Options{}, 1, nil)
	if !errors.Is(err, ErrNoFiles) {
		t.Fatalf("err = %v", err)
	}
	if summary.LogPath == "" || !strings.Contains(live.String(), "no XISF files found") {
		t.Fatalf("log %q text %s", summary.LogPath, live.String())
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 bytes"},
		{1, "1 byte"},
		{1023, "1023 bytes"},
		{1024, "1.0Kb"},
		{1536, "1.5Kb"},
		{1024 * 1024, "1.0Mb"},
		{1024 * 1024 * 1024, "1.0Gb"},
	}
	for _, tc := range cases {
		if got := formatSize(tc.n); got != tc.want {
			t.Fatalf("formatSize(%d) = %s, want %s", tc.n, got, tc.want)
		}
	}
}

func TestParseCores(t *testing.T) {
	if _, err := ParseCores(0); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := ParseCores(runtime.NumCPU() + 1); err == nil {
		t.Fatal("expected an error")
	}
	n, err := ParseCores(1)
	if err != nil || n != 1 {
		t.Fatalf("got %d %v", n, err)
	}
	if DefaultCores() < 1 || DefaultCores() > runtime.NumCPU() {
		t.Fatalf("default %d", DefaultCores())
	}
}

func TestCheckRejectsEmpty(t *testing.T) {
	if _, _, err := Check("", t.TempDir()); err == nil {
		t.Fatal("expected an error")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
