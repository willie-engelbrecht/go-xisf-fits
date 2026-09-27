// Package web serves a localhost page that runs a batch conversion.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-xisf-fits/internal/batch"
	"go-xisf-fits/internal/fits"
)

//go:embed index.html
var page embed.FS

var pageTmpl = template.Must(template.ParseFS(page, "index.html"))

type pageData struct {
	CoreChoices  []int
	DefaultCores int
}

func coreChoices() []int {
	n := runtime.NumCPU()
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

// Server converts directories from the localhost form.
type Server struct {
	mu       sync.Mutex
	dialogMu sync.Mutex
}

// Handler is the page and the convert endpoint.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			s.page(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/convert":
			s.convert(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/browse":
			s.browse(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	err := pageTmpl.Execute(w, pageData{
		CoreChoices:  coreChoices(),
		DefaultCores: batch.DefaultCores(),
	})
	if err != nil {
		http.Error(w, "page is missing", http.StatusInternalServerError)
	}
}

func (s *Server) convert(w http.ResponseWriter, r *http.Request) {
	if !s.mu.TryLock() {
		http.Error(w, "error: a conversion is already running", http.StatusConflict)
		return
	}
	defer s.mu.Unlock()

	input := strings.TrimSpace(r.FormValue("input"))
	output := strings.TrimSpace(r.FormValue("output"))
	if input == "" || output == "" {
		http.Error(w, "error: input directory and output directory are required", http.StatusBadRequest)
		return
	}
	if _, _, err := batch.Check(input, output); err != nil {
		http.Error(w, "error: "+err.Error(), http.StatusBadRequest)
		return
	}
	opt, err := fits.Parse(r.FormValue("compression"), r.FormValue("shuffle") == "on" || r.FormValue("shuffle") == "true")
	if err != nil {
		http.Error(w, "error: "+err.Error(), http.StatusBadRequest)
		return
	}
	cores := batch.DefaultCores()
	if raw := strings.TrimSpace(r.FormValue("cores")); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			http.Error(w, "error: cores must be a whole number", http.StatusBadRequest)
			return
		}
		cores, err = batch.ParseCores(n)
		if err != nil {
			http.Error(w, "error: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	live := &flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		live.f = f
		f.Flush()
	}
	_, _ = batch.Run(input, output, live, opt, cores, func(done, total int) {
		fmt.Fprintf(live, "PROGRESS %d %d\n", done, total)
	})
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	kind := r.FormValue("kind")
	var title string
	var allowNew bool
	switch kind {
	case "input":
		title = "Select the input directory"
	case "output":
		title = "Select the output directory"
		allowNew = true
	default:
		http.Error(w, "error: browse kind must be input or output", http.StatusBadRequest)
		return
	}
	if !s.dialogMu.TryLock() {
		http.Error(w, "error: a folder dialog is already open", http.StatusConflict)
		return
	}
	defer s.dialogMu.Unlock()
	path, err := pickDirectory(title, allowNew)
	if err != nil {
		http.Error(w, "error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, path)
}

// CheckAddr reports whether addr is a loopback listen address.
func CheckAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("gui listens on localhost only, not %s", host)
	}
	return nil
}

// Listen serves the page on addr until the process is stopped.
func Listen(addr string) error {
	if err := CheckAddr(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	fmt.Println("XISF to FITS at", url)
	openBrowser(url)
	srv := &http.Server{
		Handler:           (&Server{}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.Serve(ln)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
	}
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if f.f != nil {
		f.f.Flush()
	}
	return n, err
}
