// Package xisf reads PixInsight XISF images into uncompressed pixel bytes.
package xisf

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pierrec/lz4/v4"
)

// Keyword is one FITS keyword stored in the XISF header.
type Keyword struct {
	Name    string
	Value   string
	Comment string
}

// Image is one XISF image after decompression and byte unshuffling.
// Data is little-endian, plane by plane, in XISF channel order.
type Image struct {
	Axes     []int
	Channels int
	Format   string
	Keywords []Keyword
	Data     []byte
}

// SampleBytes returns the storage size of one sample.
func SampleBytes(format string) (int, error) {
	switch format {
	case "UInt8":
		return 1, nil
	case "UInt16":
		return 2, nil
	case "UInt32", "Float32":
		return 4, nil
	case "Float64":
		return 8, nil
	case "UInt64", "Complex32", "Complex64":
		return 0, fmt.Errorf("unsupported sample format %s", format)
	default:
		return 0, fmt.Errorf("unsupported sample format %s", format)
	}
}

// DataBytes is the uncompressed size of the image in bytes.
func DataBytes(img *Image) (int, error) {
	sample, err := SampleBytes(img.Format)
	if err != nil {
		return 0, err
	}
	if img.Channels < 1 {
		return 0, fmt.Errorf("invalid channel count %d", img.Channels)
	}
	if len(img.Axes) == 0 {
		return 0, fmt.Errorf("image dimensions are missing")
	}
	total := int64(1)
	for _, n := range img.Axes {
		if n < 1 {
			return 0, fmt.Errorf("invalid dimension %d", n)
		}
		total, err = mulCap(total, int64(n))
		if err != nil {
			return 0, err
		}
	}
	total, err = mulCap(total, int64(img.Channels))
	if err != nil {
		return 0, err
	}
	total, err = mulCap(total, int64(sample))
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

// mulCap multiplies a and b, rejecting results above 4 GiB.
func mulCap(a, b int64) (int64, error) {
	const maxImageBytes int64 = 4 << 30
	if a <= 0 || b <= 0 || a > maxImageBytes/b {
		return 0, fmt.Errorf("image dimensions overflow")
	}
	return a * b, nil
}

// Read opens an XISF file and returns the uncompressed image.
func Read(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sig := make([]byte, 8)
	if _, err := io.ReadFull(f, sig); err != nil {
		return nil, fmt.Errorf("read signature: %w", err)
	}
	if string(sig) != "XISF0100" {
		return nil, fmt.Errorf("incorrect XISF signature %q", sig)
	}

	var headerLen uint32
	var reserved uint32
	if err := readLE32(f, &headerLen); err != nil {
		return nil, fmt.Errorf("read header length: %w", err)
	}
	if err := readLE32(f, &reserved); err != nil {
		return nil, fmt.Errorf("read reserved field: %w", err)
	}
	if headerLen == 0 || headerLen > 32<<20 {
		return nil, fmt.Errorf("invalid XML header length %d", headerLen)
	}
	xmlBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(f, xmlBytes); err != nil {
		return nil, fmt.Errorf("read XML header: %w", err)
	}

	hdr, err := parseHeader(xmlBytes)
	if err != nil {
		return nil, err
	}
	axes, channels, err := parseGeometry(hdr.geometry)
	if err != nil {
		return nil, err
	}
	img := &Image{
		Axes:     axes,
		Channels: channels,
		Format:   hdr.format,
		Keywords: hdr.keywords,
	}
	expected, err := DataBytes(img)
	if err != nil {
		return nil, err
	}

	method, start, length, err := parseLocation(hdr.location)
	if err != nil {
		return nil, err
	}
	if method != "attachment" {
		return nil, fmt.Errorf("location method %q is not supported", method)
	}

	codec, uncompressed, itemSize, err := parseCompression(hdr.compression)
	if err != nil {
		return nil, err
	}
	if err := checkCodec(codec); err != nil {
		return nil, err
	}
	if codec == "" {
		if length != expected {
			return nil, fmt.Errorf("attachment length %d does not match image size %d", length, expected)
		}
	} else if uncompressed != expected {
		return nil, fmt.Errorf("uncompressed size %d does not match image size %d", uncompressed, expected)
	}

	if _, err := f.Seek(int64(start), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to image data: %w", err)
	}
	compressed := make([]byte, length)
	if _, err := io.ReadFull(f, compressed); err != nil {
		return nil, fmt.Errorf("read image data: %w", err)
	}
	// The checksum covers the data block as stored, before decompression.
	if err := verifyChecksum(hdr.checksum, compressed); err != nil {
		return nil, err
	}

	raw, err := decompress(codec, compressed, expected)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(codec, "+sh") {
		if itemSize == 0 {
			itemSize, err = SampleBytes(img.Format)
			if err != nil {
				return nil, err
			}
		}
		raw, err = unshuffle(raw, itemSize)
		if err != nil {
			return nil, err
		}
	}
	if len(raw) != expected {
		return nil, fmt.Errorf("decoded %d bytes, expected %d", len(raw), expected)
	}
	img.Data = raw
	return img, nil
}

func readLE32(r io.Reader, v *uint32) error {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	*v = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	return nil
}

type xmlHeader struct {
	geometry    string
	format      string
	location    string
	compression string
	checksum    string
	keywords    []Keyword
}

func parseHeader(xmlBytes []byte) (xmlHeader, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var hdr xmlHeader
	depth := 0
	imageDepth := 0
	sawImage := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlHeader{}, fmt.Errorf("parse XML header: %w", err)
		}
		switch se := tok.(type) {
		case xml.StartElement:
			depth++
			switch se.Name.Local {
			case "Image":
				if sawImage {
					break
				}
				sawImage = true
				imageDepth = depth
				hdr.geometry = attr(se, "geometry")
				hdr.format = attr(se, "sampleFormat")
				hdr.location = attr(se, "location")
				hdr.compression = attr(se, "compression")
				hdr.checksum = attr(se, "checksum")
			case "FITSKeyword":
				if !sawImage || imageDepth == 0 || depth <= imageDepth {
					break
				}
				hdr.keywords = append(hdr.keywords, Keyword{
					Name:    attr(se, "name"),
					Value:   attr(se, "value"),
					Comment: attr(se, "comment"),
				})
			}
		case xml.EndElement:
			if se.Name.Local == "Image" && depth == imageDepth {
				imageDepth = 0
			}
			depth--
		}
	}
	if !sawImage {
		return xmlHeader{}, fmt.Errorf("XISF image header is missing")
	}
	if hdr.geometry == "" || hdr.format == "" || hdr.location == "" {
		return xmlHeader{}, fmt.Errorf("XISF image header is missing geometry, sample format, or location")
	}
	return hdr, nil
}

func attr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func parseGeometry(s string) ([]int, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return nil, 0, fmt.Errorf("invalid geometry %q", s)
	}
	channels, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || channels < 1 {
		return nil, 0, fmt.Errorf("invalid geometry %q", s)
	}
	axes := make([]int, 0, len(parts)-1)
	for _, p := range parts[:len(parts)-1] {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, 0, fmt.Errorf("invalid geometry %q", s)
		}
		axes = append(axes, n)
	}
	return axes, channels, nil
}

func parseLocation(s string) (method string, start int, length int, err error) {
	parts := strings.Split(s, ":")
	method = parts[0]
	if method != "attachment" {
		return method, 0, 0, nil
	}
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("invalid location %q", s)
	}
	start64, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || start64 < 0 {
		return "", 0, 0, fmt.Errorf("invalid location %q", s)
	}
	length64, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || length64 < 1 {
		return "", 0, 0, fmt.Errorf("invalid location %q", s)
	}
	if start64 > int64(^uint(0)>>1) || length64 > int64(^uint(0)>>1) {
		return "", 0, 0, fmt.Errorf("invalid location %q", s)
	}
	return method, int(start64), int(length64), nil
}

func parseCompression(s string) (codec string, uncompressed int, itemSize int, err error) {
	if s == "" {
		return "", 0, 0, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", 0, 0, fmt.Errorf("invalid compression %q", s)
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || size < 1 || size > int64(^uint(0)>>1) {
		return "", 0, 0, fmt.Errorf("invalid compression %q", s)
	}
	if len(parts) == 3 {
		itemSize, err = strconv.Atoi(parts[2])
		if err != nil || itemSize < 1 {
			return "", 0, 0, fmt.Errorf("invalid compression %q", s)
		}
	}
	return parts[0], int(size), itemSize, nil
}

func checkCodec(codec string) error {
	switch codec {
	case "", "zlib", "zlib+sh", "lz4", "lz4+sh", "lz4hc", "lz4hc+sh":
		return nil
	default:
		return fmt.Errorf("unsupported compression codec %s", codec)
	}
}

func decompress(codec string, src []byte, size int) ([]byte, error) {
	base := strings.TrimSuffix(codec, "+sh")
	switch base {
	case "":
		return src, nil
	case "zlib":
		zr, err := zlib.NewReader(bytes.NewReader(src))
		if err != nil {
			return nil, fmt.Errorf("zlib decompress: %w", err)
		}
		defer zr.Close()
		dst, err := io.ReadAll(io.LimitReader(zr, int64(size)+1))
		if err != nil {
			return nil, fmt.Errorf("zlib decompress: %w", err)
		}
		if len(dst) != size {
			return nil, fmt.Errorf("zlib decompressed %d bytes, expected %d", len(dst), size)
		}
		return dst, nil
	case "lz4", "lz4hc":
		dst := make([]byte, size)
		n, err := lz4.UncompressBlock(src, dst)
		if err != nil {
			return nil, fmt.Errorf("%s decompress: %w", base, err)
		}
		if n != size {
			return nil, fmt.Errorf("%s decompressed %d bytes, expected %d", base, n, size)
		}
		return dst, nil
	default:
		return nil, fmt.Errorf("unsupported compression codec %s", codec)
	}
}

// unshuffle reverses XISF byte shuffling. Byte j of item i is stored at
// shuffled index j*itemCount+i.
func unshuffle(data []byte, itemSize int) ([]byte, error) {
	if itemSize <= 1 {
		return data, nil
	}
	if len(data)%itemSize != 0 {
		return nil, fmt.Errorf("shuffled data length %d is not a multiple of item size %d", len(data), itemSize)
	}
	n := len(data) / itemSize
	out := make([]byte, len(data))
	for j := 0; j < itemSize; j++ {
		plane := data[j*n : (j+1)*n]
		for i := 0; i < n; i++ {
			out[i*itemSize+j] = plane[i]
		}
	}
	return out, nil
}

func verifyChecksum(spec string, data []byte) error {
	if spec == "" {
		return nil
	}
	kind, hexSum, ok := strings.Cut(spec, ":")
	if !ok || kind != "sha-256" || hexSum == "" {
		return fmt.Errorf("unsupported checksum %q", spec)
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), hexSum) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}
