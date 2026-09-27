package xisf

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnshuffle(t *testing.T) {
	shuffled := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	got, err := unshuffle(shuffled, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x11, 0x44, 0x22, 0x55, 0x33, 0x66}
	if !bytes.Equal(got, want) {
		t.Fatalf("unshuffle = %x, want %x", got, want)
	}
	same, err := unshuffle(shuffled, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same, shuffled) {
		t.Fatal("item size 1 changed the buffer")
	}
}

func TestReadFixtures(t *testing.T) {
	root := repoRoot(t)
	images := filepath.Join(root, "tests", "images")
	cases := []struct {
		name     string
		axes     []int
		channels int
		format   string
	}{
		{"xisf-image-gray-256x256-8bits.xisf", []int{256, 256}, 1, "UInt8"},
		{"xisf-image-gray-256x256-16bits-zlib.xisf", []int{256, 256}, 1, "UInt16"},
		{"xisf-image-gray-256x256-16bits-zlib_sh.xisf", []int{256, 256}, 1, "UInt16"},
		{"xisf-image-gray-256x256-float-32bits.xisf", []int{255, 255}, 1, "Float32"},
		{"xisf-image-gray-256x256-float-64bits.xisf", []int{255, 255}, 1, "Float64"},
		{"xisf-image-rgb-256x256-8bits.xisf", []int{256, 256}, 3, "UInt8"},
		{"xisf-image-rgb-256x256-16bits.xisf", []int{256, 256}, 3, "UInt16"},
		{"xisf-image-rgb-256x256-32bits.xisf", []int{256, 256}, 3, "UInt32"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, err := Read(filepath.Join(images, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if img.Format != tc.format || img.Channels != tc.channels || !sameInts(img.Axes, tc.axes) {
				t.Fatalf("got %s %d channels %v", img.Format, img.Channels, img.Axes)
			}
			n, err := DataBytes(img)
			if err != nil {
				t.Fatal(err)
			}
			if len(img.Data) != n {
				t.Fatalf("data length %d, want %d", len(img.Data), n)
			}
		})
	}
}

func TestReadEmbeddedLocation(t *testing.T) {
	path := filepath.Join(repoRoot(t), "tests", "images", "xisf-image-rgb-256x256-8bits-compressed.xisf")
	_, err := Read(path)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("embedded")) {
		t.Fatalf("error = %v, want location method embedded", err)
	}
}

func TestNINAChecksum(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "2026-08-13_*.xisf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("NINA sample not present")
	}
	img, err := Read(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if img.Format != "UInt16" || img.Channels != 1 || !sameInts(img.Axes, []int{9576, 6388}) {
		t.Fatalf("got %s channels=%d axes=%v", img.Format, img.Channels, img.Axes)
	}
	if len(img.Keywords) == 0 {
		t.Fatal("expected FITS keywords")
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestSampleBytesRejectsComplex(t *testing.T) {
	if _, err := SampleBytes("Complex64"); err == nil {
		t.Fatal("expected an error")
	}
}
