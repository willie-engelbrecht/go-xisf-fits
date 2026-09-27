package fits

import (
	"fmt"
	"strings"
)

// Options selects how image pixels are stored in the FITS file.
// Compression names follow the FITS tiled-image convention:
// none and gzip (GZIP_1). Shuffle with gzip selects GZIP_2.
// An empty Compression is uncompressed, which is the zero value.
type Options struct {
	Compression string
	Shuffle     bool
}

// Parse checks a compression name and the byte-shuffle flag.
// An empty compression name means uncompressed.
func Parse(compression string, shuffle bool) (Options, error) {
	opt := Options{Compression: compression, Shuffle: shuffle}
	if _, err := opt.kind(); err != nil {
		return Options{}, err
	}
	return opt, nil
}

func (o Options) kind() (string, error) {
	c := strings.ToLower(strings.TrimSpace(o.Compression))
	if c == "" {
		c = "none"
	}
	switch c {
	case "none", "gzip":
	default:
		return "", fmt.Errorf("compression must be none or gzip")
	}
	if o.Shuffle && c != "gzip" {
		return "", fmt.Errorf("byte shuffling is only available with gzip compression")
	}
	return c, nil
}

// Validate reports an unsupported compression choice.
func (o Options) Validate() error {
	_, err := o.kind()
	return err
}

// Describe is a short label for the log.
func (o Options) Describe() string {
	c, err := o.kind()
	if err != nil {
		return o.Compression
	}
	switch c {
	case "gzip":
		if o.Shuffle {
			return "gzip with byte shuffling (GZIP_2)"
		}
		return "gzip (GZIP_1)"
	default:
		return "none"
	}
}
