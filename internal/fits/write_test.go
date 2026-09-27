package fits

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"xisfits/internal/xisf"
)

func TestWriteUInt8PaddingAndKeyword(t *testing.T) {
	img := &xisf.Image{
		Axes:     []int{1, 1},
		Channels: 1,
		Format:   "UInt8",
		Data:     []byte{42},
		Keywords: []xisf.Keyword{{
			Name:    "OBJECT",
			Value:   "'Test'",
			Comment: strings.Repeat("x", 200),
		}},
	}
	path := filepath.Join(t.TempDir(), "tiny.fits")
	if err := Write(path, img, Options{}); err != nil {
		t.Fatal(err)
	}
	header, data := splitFITS(t, path)
	if len(header)%2880 != 0 {
		t.Fatalf("header length %d", len(header))
	}
	requireCard(t, header, "SIMPLE", "T")
	requireCard(t, header, "BITPIX", "8")
	requireCard(t, header, "NAXIS", "2")
	requireCard(t, header, "BZERO", "0")
	requireCard(t, header, "OBJECT", "'Test'")
	if data[0] != 42 {
		t.Fatalf("first data byte %d", data[0])
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size()%2880 != 0 {
		t.Fatalf("file size %d", info.Size())
	}
}

func TestUnsignedOffsets(t *testing.T) {
	u16 := &xisf.Image{
		Axes:     []int{2},
		Channels: 1,
		Format:   "UInt16",
		Data:     make([]byte, 4),
	}
	binary.LittleEndian.PutUint16(u16.Data[0:], 0)
	binary.LittleEndian.PutUint16(u16.Data[2:], 65535)
	path := filepath.Join(t.TempDir(), "u16.fits")
	if err := Write(path, u16, Options{}); err != nil {
		t.Fatal(err)
	}
	header, data := splitFITS(t, path)
	if data[0] != 0x80 || data[1] != 0x00 || data[2] != 0x7f || data[3] != 0xff {
		t.Fatalf("uint16 bytes %x", data[:4])
	}
	requireCard(t, header, "BITPIX", "16")
	requireCard(t, header, "BZERO", "32768")

	u32 := &xisf.Image{
		Axes:     []int{1},
		Channels: 1,
		Format:   "UInt32",
		Data:     make([]byte, 4),
	}
	binary.LittleEndian.PutUint32(u32.Data, 0)
	path = filepath.Join(t.TempDir(), "u32.fits")
	if err := Write(path, u32, Options{}); err != nil {
		t.Fatal(err)
	}
	header, data = splitFITS(t, path)
	if data[0] != 0x80 || data[1] != 0 || data[2] != 0 || data[3] != 0 {
		t.Fatalf("uint32 bytes %x", data[:4])
	}
	requireCard(t, header, "BZERO", "2147483648.0")

	f32 := &xisf.Image{
		Axes:     []int{1},
		Channels: 1,
		Format:   "Float32",
		Data:     []byte{0x00, 0x00, 0x80, 0x3f},
	}
	path = filepath.Join(t.TempDir(), "f32.fits")
	if err := Write(path, f32, Options{}); err != nil {
		t.Fatal(err)
	}
	header, data = splitFITS(t, path)
	if data[0] != 0x3f || data[1] != 0x80 || data[2] != 0 || data[3] != 0 {
		t.Fatalf("float32 bytes %x", data[:4])
	}
	requireCard(t, header, "BITPIX", "-32")

	f64 := &xisf.Image{
		Axes:     []int{1},
		Channels: 1,
		Format:   "Float64",
		Data:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f},
	}
	path = filepath.Join(t.TempDir(), "f64.fits")
	if err := Write(path, f64, Options{}); err != nil {
		t.Fatal(err)
	}
	_, data = splitFITS(t, path)
	if data[0] != 0x3f || data[1] != 0xf0 || data[2] != 0 || data[3] != 0 || data[4] != 0 || data[5] != 0 || data[6] != 0 || data[7] != 0 {
		t.Fatalf("float64 bytes %x", data[:8])
	}
}

func TestWriteFixtures(t *testing.T) {
	images := filepath.Join(repoRoot(t), "tests", "images")
	cases := []struct {
		name   string
		bitpix string
		naxis3 bool
	}{
		{"xisf-image-gray-256x256-8bits.xisf", "8", false},
		{"xisf-image-gray-256x256-16bits-zlib_sh.xisf", "16", false},
		{"xisf-image-rgb-256x256-8bits.xisf", "8", true},
		{"xisf-image-rgb-256x256-32bits.xisf", "32", true},
		{"xisf-image-gray-256x256-float-64bits.xisf", "-64", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, err := xisf.Read(filepath.Join(images, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "out.fits")
			if err := Write(path, img, Options{}); err != nil {
				t.Fatal(err)
			}
			header, data := splitFITS(t, path)
			requireCard(t, header, "BITPIX", tc.bitpix)
			if tc.naxis3 {
				requireCard(t, header, "NAXIS3", "3")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size()%2880 != 0 {
				t.Fatalf("file size %d", info.Size())
			}
			if len(data) < len(img.Data) {
				t.Fatalf("data %d shorter than image %d", len(data), len(img.Data))
			}
		})
	}
}

func TestNINASampleFITS(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "2026-08-13_*.xisf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("NINA sample not present")
	}
	img, err := xisf.Read(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nina.fits")
	if err := Write(path, img, Options{}); err != nil {
		t.Fatal(err)
	}
	header, data := splitFITS(t, path)
	requireCard(t, header, "BITPIX", "16")
	requireCard(t, header, "NAXIS", "2")
	requireCard(t, header, "NAXIS1", "9576")
	requireCard(t, header, "NAXIS2", "6388")
	requireCard(t, header, "BZERO", "32768")
	requireCard(t, header, "BSCALE", "1")
	requireCard(t, header, "OBJECT", "'Fireworks Galaxy'")
	if len(data) < 9576*6388*2 {
		t.Fatalf("data length %d", len(data))
	}
}

func requireCard(t *testing.T, header, key, want string) {
	t.Helper()
	for i := 0; i+80 <= len(header); i += 80 {
		card := header[i : i+80]
		if strings.TrimSpace(card[:8]) != key {
			continue
		}
		rest := strings.TrimSpace(card[10:])
		if j := strings.Index(rest, " /"); j >= 0 {
			rest = strings.TrimSpace(rest[:j])
		}
		if rest != want {
			t.Fatalf("card %s = %q, want %q", key, rest, want)
		}
		return
	}
	t.Fatalf("card %s not found", key)
}

func splitFITS(t *testing.T, path string) (string, []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b)%2880 != 0 {
		t.Fatalf("file size %d is not a multiple of 2880", len(b))
	}
	i := 0
	for i+80 <= len(b) {
		card := b[i : i+80]
		i += 80
		if bytesHasPrefix(card, []byte("END")) {
			break
		}
		if i > 100*2880 {
			t.Fatal("END card not found")
		}
	}
	if i%2880 != 0 {
		i += 2880 - i%2880
	}
	return string(b[:i]), b[i:]
}

func bytesHasPrefix(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
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
