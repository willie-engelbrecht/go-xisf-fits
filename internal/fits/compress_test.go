package fits

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"go-xisf-fits/internal/xisf"
)

func TestGzipShuffleRoundTrip(t *testing.T) {
	raw := noisy(64, 2)
	shuffled := shuffleMSB(raw, 2)
	if bytes.Equal(shuffled, raw) {
		t.Fatal("shuffle did not change bytes")
	}
	if !bytes.Equal(unshuffleMSB(shuffled, 2), raw) {
		t.Fatal("unshuffle")
	}
	comp, err := gzipBytes(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	back, err := gunzipBytes(comp, len(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unshuffleMSB(back, 2), raw) {
		t.Fatal("gzip shuffle round trip")
	}
}

func TestWriteCompressedHeaders(t *testing.T) {
	data := make([]byte, 4*3*2)
	for i := range data {
		data[i] = byte(i * 3)
	}
	img := &xisf.Image{
		Axes:     []int{4, 3},
		Channels: 1,
		Format:   "UInt16",
		Data:     data,
		Keywords: []xisf.Keyword{{Name: "FILTER", Value: "'R       '"}},
	}
	for _, tc := range []struct {
		opt  Options
		want string
	}{
		{Options{Compression: "gzip"}, "GZIP_1"},
		{Options{Compression: "gzip", Shuffle: true}, "GZIP_2"},
	} {
		path := t.TempDir() + "/out.fits"
		if err := Write(path, img, tc.opt); err != nil {
			t.Fatal(err)
		}
		file, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(file)
		if !strings.Contains(text, "ZCMPTYPE= '"+tc.want+"'") {
			t.Fatalf("%s missing ZCMPTYPE", tc.want)
		}
		if !strings.Contains(text, "ZIMAGE  =                    T") {
			t.Fatalf("%s missing ZIMAGE", tc.want)
		}
		payload, _, _, err := encode(img)
		if err != nil {
			t.Fatal(err)
		}
		heap := fitsHeap(t, file)
		var got []byte
		switch tc.want {
		case "GZIP_1":
			got, err = gunzipBytes(heap, len(payload))
		case "GZIP_2":
			got, err = gunzipBytes(heap, len(payload))
			if err == nil {
				got = unshuffleMSB(got, 2)
			}
		}
		if err != nil {
			t.Fatal(tc.want, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s pixels differ", tc.want)
		}
	}
}

func TestParseShuffle(t *testing.T) {
	if _, err := Parse("rice", false); err == nil || !strings.Contains(err.Error(), "gzip") {
		t.Fatalf("got %v", err)
	}
	if _, err := Parse("none", true); err == nil {
		t.Fatal("expected shuffle error")
	}
	opt, err := Parse("gzip", true)
	if err != nil {
		t.Fatal(err)
	}
	if opt.Describe() != "gzip with byte shuffling (GZIP_2)" {
		t.Fatal(opt.Describe())
	}
}

func noisy(n, width int) []byte {
	out := make([]byte, n*width)
	x := uint32(0x12345678)
	for i := range out {
		x = x*1664525 + 1013904223
		out[i] = byte(x >> 16)
	}
	return out
}

func fitsHeap(t *testing.T, file []byte) []byte {
	t.Helper()
	off := skipHeader(t, file, 0)
	off = skipHeader(t, file, off)
	if off+8 > len(file) {
		t.Fatal("truncated table")
	}
	n := int(file[off])<<24 | int(file[off+1])<<16 | int(file[off+2])<<8 | int(file[off+3])
	start := off + 8
	if start+n > len(file) {
		t.Fatalf("heap %d exceeds file", n)
	}
	return file[start : start+n]
}

func skipHeader(t *testing.T, file []byte, off int) int {
	t.Helper()
	for {
		if off+80 > len(file) {
			t.Fatal("truncated header")
		}
		card := string(file[off : off+80])
		off += 80
		if strings.HasPrefix(card, "END") {
			if rem := off % 2880; rem != 0 {
				off += 2880 - rem
			}
			return off
		}
	}
}
