# XISF to FITS

Go XISF to FITS converts PixInsight [XISF](https://pixinsight.com/doc/docs/XISF-1.0-spec/XISF-1.0-spec.html) images to [FITS](https://fits.gsfc.nasa.gov/fits_standard.html).

go-xisf-fits walks an input directory, including subfolders, and writes a `.fits` file for each `.xisf` file. The output directory mirrors that folder layout and is created if it is missing. A log named `YYYY_MM_DD-HH-MM-SS_output.txt` is written in the output directory. If the output tree sits inside the input tree, it is skipped so a previous run is not converted again.

## How to run

Go is required (`go 1.27.1` in `go.mod`).

Convert a directory:

```bash
go run ./cmd convert --input DIR --output DIR
```

Open the GUI, which serves a page on localhost and opens a browser:

```bash
go run ./cmd gui
```

Or build once and run the binary:

```bash
go build -o go-xisf-fits ./cmd
```

On Windows the binary is `go-xisf-fits.exe`.

```bash
go-xisf-fits convert --input DIR --output DIR
go-xisf-fits gui
```

`go-xisf-fits` with no arguments, or `go-xisf-fits help`, prints the same usage.

## SIMD

Pixel bytes are reordered with a scalar loop unless you build with Go's experimental SIMD support. Set `GOEXPERIMENT` to `simd` in the same shell, then build or run as usual. That shell keeps the setting until you close it.

Windows (PowerShell):

```powershell
$env:GOEXPERIMENT = "simd"
go build -o go-xisf-fits.exe ./cmd
```

Linux:

```bash
export GOEXPERIMENT=simd
go build -o go-xisf-fits ./cmd
```

`go run` in that same shell uses the SIMD build too. A normal build, with the variable unset, uses the scalar path.

## Command-line options

### convert

```bash
go-xisf-fits convert --input DIR --output DIR
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--input DIR` | required | Directory to walk, including subfolders. Must already exist. |
| `--output DIR` | required | Where `.fits` files are written, mirroring the input folders. Created if missing. |
| `--compression none\|gzip` | `gzip` | How pixels are stored. `gzip` is FITS `GZIP_1`, or `GZIP_2` when byte shuffling is on. |
| `--shuffle` | on | Byte-shuffle before gzip (FITS `GZIP_2`). Turn it off with `--shuffle=false`. Only valid with gzip. When compression is `none` and this flag is left unset, shuffling is turned off. |
| `--cores N` | half the logical CPUs | How many files are converted at once. Allowed range is 1 through the machine's CPU count. |

### gui

```bash
go-xisf-fits gui [--port 8080]
```

The page has the same input, output, compression, shuffle, and cores controls, plus a live log and a progress bar. Folder browse dialogs work on Windows. On other systems, type the paths into the form.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--port N` | `8080` | Listen port on `127.0.0.1`. Must be from 1 to 65535. |
| `--addr HOST:PORT` | | Full loopback address, instead of `--port`. Pass one of these, not both. The server listens on localhost only. |

## Attribution

This project is based on [XISFITS](https://github.com/vrruiz/xisfits) by Víctor R. Ruiz.

Licensed under the MIT License. See [LICENSE](LICENSE).
