package fits

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"go-xisf-fits/internal/xisf"
)

func writeCompressed(path string, axes []int, bitpix int, bzero string, keywords []xisf.Keyword, payload []byte, opt Options) error {
	kind, err := opt.kind()
	if err != nil {
		return err
	}
	width := abs(bitpix) / 8
	tile := payload
	zcmp := "GZIP_1"
	switch kind {
	case "gzip":
		if opt.Shuffle {
			zcmp = "GZIP_2"
			if width > 1 {
				tile = shuffleMSB(payload, width)
			}
		}
		tile, err = gzipBytes(tile)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("compression must be none or gzip")
	}
	if len(tile) > int(^uint32(0)>>1) {
		return fmt.Errorf("compressed tile is too large")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	if err := writePrimary(bw); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := writeExtension(bw, axes, bitpix, bzero, zcmp, keywords, tile); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func writePrimary(w io.Writer) error {
	cards := []string{
		valueCard("SIMPLE", "T"),
		intCard("BITPIX", 8),
		intCard("NAXIS", 0),
		valueCard("EXTEND", "T"),
		pad("END"),
	}
	return writeCards(w, cards)
}

func writeExtension(w io.Writer, axes []int, bitpix int, bzero, zcmp string, keywords []xisf.Keyword, heap []byte) error {
	cards := []string{
		stringCard("XTENSION", "'BINTABLE'"),
		intCard("BITPIX", 8),
		intCard("NAXIS", 2),
		intCard("NAXIS1", 8),
		intCard("NAXIS2", 1),
		intCard("PCOUNT", len(heap)),
		intCard("GCOUNT", 1),
		intCard("TFIELDS", 1),
		stringCard("TTYPE1", "'COMPRESSED_DATA'"),
		stringCard("TFORM1", "'1PB'"),
		valueCard("ZIMAGE", "T"),
		stringCard("ZCMPTYPE", "'"+zcmp+"'"),
		intCard("ZBITPIX", bitpix),
		intCard("ZNAXIS", len(axes)),
	}
	for i, axis := range axes {
		cards = append(cards,
			intCard(fmt.Sprintf("ZNAXIS%d", i+1), axis),
			intCard(fmt.Sprintf("ZTILE%d", i+1), axis),
		)
	}
	cards = append(cards, valueCard("BSCALE", "1"), valueCard("BZERO", bzero))
	for _, kw := range keywords {
		if skipCompressedKeyword(kw.Name) {
			continue
		}
		cards = append(cards, keywordCard(kw))
	}
	cards = append(cards, pad("END"))
	if err := writeCards(w, cards); err != nil {
		return err
	}
	var desc [8]byte
	binary.BigEndian.PutUint32(desc[0:4], uint32(len(heap)))
	binary.BigEndian.PutUint32(desc[4:8], 0)
	if _, err := w.Write(desc[:]); err != nil {
		return err
	}
	if _, err := w.Write(heap); err != nil {
		return err
	}
	// The table row and the heap together form the data block.
	total := 8 + len(heap)
	if rem := total % 2880; rem != 0 {
		if _, err := w.Write(make([]byte, 2880-rem)); err != nil {
			return err
		}
	}
	return nil
}

func skipCompressedKeyword(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	switch upper {
	case "SIMPLE", "XTENSION", "BITPIX", "NAXIS", "PCOUNT", "GCOUNT", "TFIELDS",
		"EXTEND", "END", "BSCALE", "BZERO", "ZIMAGE", "ZCMPTYPE", "ZBITPIX", "ZNAXIS":
		return true
	default:
		if strings.HasPrefix(upper, "NAXIS") && len(upper) > len("NAXIS") {
			return true
		}
		if strings.HasPrefix(upper, "ZNAXIS") || strings.HasPrefix(upper, "ZTILE") ||
			strings.HasPrefix(upper, "TFORM") || strings.HasPrefix(upper, "TTYPE") ||
			strings.HasPrefix(upper, "ZNAME") || strings.HasPrefix(upper, "ZVAL") {
			return true
		}
		return false
	}
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipBytes(data []byte, size int) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, int64(size)+1))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	if len(out) != size {
		return nil, fmt.Errorf("gzip decompressed %d bytes, expected %d", len(out), size)
	}
	return out, nil
}

// shuffleMSB groups big-endian pixel bytes so the most significant byte of
// every pixel comes first. width is the number of bytes per pixel.
func shuffleMSB(raw []byte, width int) []byte {
	if width <= 1 {
		return append([]byte(nil), raw...)
	}
	n := len(raw) / width
	out := make([]byte, len(raw))
	for p := 0; p < width; p++ {
		for i := 0; i < n; i++ {
			out[p*n+i] = raw[i*width+p]
		}
	}
	return out
}

func unshuffleMSB(raw []byte, width int) []byte {
	if width <= 1 {
		return append([]byte(nil), raw...)
	}
	n := len(raw) / width
	out := make([]byte, len(raw))
	for p := 0; p < width; p++ {
		for i := 0; i < n; i++ {
			out[i*width+p] = raw[p*n+i]
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
