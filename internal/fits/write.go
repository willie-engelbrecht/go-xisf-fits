// Package fits writes a FITS primary HDU from an XISF image.
package fits

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"xisfits/internal/xisf"
)

// Write creates a FITS file for img, replacing path if it already exists.
// Unsigned samples are stored with a BZERO offset so the full range is kept.
// opt selects uncompressed pixels or Gzip. Gzip with Shuffle uses GZIP_2.
func Write(path string, img *xisf.Image, opt Options) error {
	if img == nil {
		return fmt.Errorf("image is missing")
	}
	if _, err := opt.kind(); err != nil {
		return err
	}
	expected, err := xisf.DataBytes(img)
	if err != nil {
		return err
	}
	if len(img.Data) != expected {
		return fmt.Errorf("decoded %d bytes, expected %d", len(img.Data), expected)
	}
	payload, bitpix, bzero, err := encode(img)
	if err != nil {
		return err
	}
	axes, err := outputAxes(img)
	if err != nil {
		return err
	}
	kind, err := opt.kind()
	if err != nil {
		return err
	}
	if kind != "none" {
		return writeCompressed(path, axes, bitpix, bzero, img.Keywords, payload, opt)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	writeErr := writeHDU(bw, axes, bitpix, bzero, img.Keywords, payload)
	if writeErr != nil {
		f.Close()
		os.Remove(path)
		return writeErr
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

func outputAxes(img *xisf.Image) ([]int, error) {
	if len(img.Axes) == 0 || img.Channels < 1 {
		return nil, fmt.Errorf("invalid image geometry")
	}
	axes := append([]int{}, img.Axes...)
	if img.Channels > 1 {
		axes = append(axes, img.Channels)
	}
	return axes, nil
}

func encode(img *xisf.Image) (payload []byte, bitpix int, bzero string, err error) {
	switch img.Format {
	case "UInt8":
		return img.Data, 8, "0", nil
	case "UInt16":
		if len(img.Data)%2 != 0 {
			return nil, 0, "", fmt.Errorf("UInt16 data length %d is odd", len(img.Data))
		}
		payload = make([]byte, len(img.Data))
		// FITS stores unsigned 16-bit samples as signed values with BZERO 32768.
		// Flipping the high bit is the two's-complement form of subtracting 32768.
		reorderU16(payload, img.Data)
		return payload, 16, "32768", nil
	case "UInt32":
		if len(img.Data)%4 != 0 {
			return nil, 0, "", fmt.Errorf("UInt32 data length %d is not a multiple of 4", len(img.Data))
		}
		payload = make([]byte, len(img.Data))
		reorderU32(payload, img.Data)
		// 2147483648 does not fit in a FITS signed integer, so BZERO is a float.
		return payload, 32, "2147483648.0", nil
	case "Float32":
		if len(img.Data)%4 != 0 {
			return nil, 0, "", fmt.Errorf("Float32 data length %d is not a multiple of 4", len(img.Data))
		}
		payload = make([]byte, len(img.Data))
		reorderF32(payload, img.Data)
		return payload, -32, "0", nil
	case "Float64":
		if len(img.Data)%8 != 0 {
			return nil, 0, "", fmt.Errorf("Float64 data length %d is not a multiple of 8", len(img.Data))
		}
		payload = make([]byte, len(img.Data))
		reorderF64(payload, img.Data)
		return payload, -64, "0", nil
	default:
		return nil, 0, "", fmt.Errorf("unsupported sample format %s", img.Format)
	}
}

func writeCards(w io.Writer, cards []string) error {
	n := 0
	for _, card := range cards {
		if len(card) != 80 {
			return fmt.Errorf("FITS card length %d", len(card))
		}
		if _, err := io.WriteString(w, card); err != nil {
			return err
		}
		n += 80
	}
	if rem := n % 2880; rem != 0 {
		if _, err := w.Write(bytesRepeat(' ', 2880-rem)); err != nil {
			return err
		}
	}
	return nil
}

func writeHDU(w *bufio.Writer, axes []int, bitpix int, bzero string, keywords []xisf.Keyword, payload []byte) error {
	n := 0
	write := func(card string) error {
		if len(card) != 80 {
			return fmt.Errorf("FITS card length %d", len(card))
		}
		if _, err := w.WriteString(card); err != nil {
			return err
		}
		n += 80
		return nil
	}
	cards := []string{
		valueCard("SIMPLE", "T"),
		intCard("BITPIX", bitpix),
		intCard("NAXIS", len(axes)),
	}
	for i, axis := range axes {
		cards = append(cards, intCard(fmt.Sprintf("NAXIS%d", i+1), axis))
	}
	cards = append(cards, valueCard("BSCALE", "1"), valueCard("BZERO", bzero))
	for _, kw := range keywords {
		if skipKeyword(kw.Name) {
			continue
		}
		cards = append(cards, keywordCard(kw))
	}
	cards = append(cards, pad("END"))
	for _, card := range cards {
		if err := write(card); err != nil {
			return err
		}
	}
	if rem := n % 2880; rem != 0 {
		if _, err := w.Write(bytesRepeat(' ', 2880-rem)); err != nil {
			return err
		}
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if rem := len(payload) % 2880; rem != 0 {
		if _, err := w.Write(make([]byte, 2880-rem)); err != nil {
			return err
		}
	}
	return nil
}

func skipKeyword(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	switch upper {
	case "SIMPLE", "BITPIX", "NAXIS", "BSCALE", "BZERO", "END":
		return true
	default:
		if strings.HasPrefix(upper, "NAXIS") && len(upper) > len("NAXIS") {
			return true
		}
		return false
	}
}

func intCard(key string, v int) string {
	return pad(fmt.Sprintf("%-8s= %20d", key, v))
}

func valueCard(key, value string) string {
	return pad(fmt.Sprintf("%-8s= %20s", key, value))
}

func stringCard(key, quoted string) string {
	return pad(fmt.Sprintf("%-8s= %s", key, quoted))
}

func keywordCard(kw xisf.Keyword) string {
	name := kw.Name
	if len(name) > 8 {
		name = name[:8]
	}
	name = strings.ToUpper(name)
	upper := strings.TrimSpace(name)
	if upper == "COMMENT" || upper == "HISTORY" {
		text := kw.Comment
		if text == "" {
			text = kw.Value
		}
		return pad(fmt.Sprintf("%-8s%s", upper, text))
	}
	body := fmt.Sprintf("%-8s= %s", name, kw.Value)
	if kw.Comment != "" {
		full := body + " / " + kw.Comment
		if len(full) <= 80 {
			return pad(full)
		}
		room := 80 - len(body) - len(" / ")
		if room > 0 {
			comment := kw.Comment
			if len(comment) > room {
				comment = comment[:room]
			}
			return pad(body + " / " + comment)
		}
	}
	return pad(body)
}

func pad(s string) string {
	if len(s) > 80 {
		s = s[:80]
	}
	if len(s) == 80 {
		return s
	}
	return s + strings.Repeat(" ", 80-len(s))
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
