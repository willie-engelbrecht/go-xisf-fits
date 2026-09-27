package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"xisfits/internal/batch"
	"xisfits/internal/fits"
	"xisfits/internal/web"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "convert":
		return runConvert(args[1:])
	case "gui":
		return runGUI(args[1:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `XISF to FITS converter

Usage:
  xisfits convert --input DIR --output DIR
  xisfits gui [--port 8080]

convert walks DIR, including subfolders, and writes a .fits file for each
.xisf file. The output directory mirrors those folders. Both directories
are required. A log file YYYY_MM_DD-HH-MM-SS_output.txt is written in the
output directory.

  --compression none|gzip   how pixels are stored (default gzip)
  --shuffle                  byte-shuffle before gzip (default on, FITS GZIP_2)
  --cores N                  files converted at once (default half the CPUs)

gui serves a page on localhost and opens a browser.
  --port N                   listen port on 127.0.0.1 (default 8080)
  --addr HOST:PORT           full loopback address, instead of --port
`)
}

func runConvert(args []string) int {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "", "input directory")
	output := fs.String("output", "", "output directory")
	compression := fs.String("compression", "gzip", "none or gzip")
	shuffle := fs.Bool("shuffle", true, "byte-shuffle before gzip")
	cores := fs.Int("cores", batch.DefaultCores(), "files converted at once")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(os.Stdout)
			return 0
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "error: --input and --output are required")
		return 1
	}
	useShuffle := *shuffle
	shuffleSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "shuffle" {
			shuffleSet = true
		}
	})
	if strings.EqualFold(strings.TrimSpace(*compression), "none") && !shuffleSet {
		useShuffle = false
	}
	opt, err := fits.Parse(*compression, useShuffle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	n, err := batch.ParseCores(*cores)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	summary, err := batch.Run(*input, *output, os.Stdout, opt, n, nil)
	if err != nil && summary.LogPath == "" {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err != nil || summary.Failed > 0 {
		return 1
	}
	return 0
}

func runGUI(args []string) int {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", "", "full loopback listen address")
	port := fs.Int("port", 8080, "listen port on 127.0.0.1")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(os.Stdout)
			return 0
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	addrSet, portSet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "addr":
			addrSet = true
		case "port":
			portSet = true
		}
	})
	listen, err := guiListenAddr(*addr, *port, addrSet, portSet)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := web.Listen(listen); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func guiListenAddr(addr string, port int, addrSet, portSet bool) (string, error) {
	if addrSet && portSet {
		return "", errors.New("pass --port or --addr, not both")
	}
	if addrSet {
		if strings.TrimSpace(addr) == "" {
			return "", errors.New("--addr is required")
		}
		return addr, nil
	}
	if port < 1 || port > 65535 {
		return "", errors.New("--port must be between 1 and 65535")
	}
	return fmt.Sprintf("127.0.0.1:%d", port), nil
}
