package web

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go-xisf-fits/internal/batch"
)

func TestConvertRejectsEmptyPaths(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/convert", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "required") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestConvertStreamsBatch(t *testing.T) {
	root := repoRoot(t)
	src := filepath.Join(root, "tests", "images", "xisf-image-gray-256x256-8bits.xisf")
	in := t.TempDir()
	out := t.TempDir()
	dst := filepath.Join(in, "nested", "frame.xisf")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()
	resp, err := http.PostForm(srv.URL+"/convert", url.Values{
		"input":  {in},
		"output": {out},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "OK ") || !strings.Contains(string(body), "1 succeeded, 0 failed") || !strings.Contains(string(body), "PROGRESS 1 1") {
		t.Fatalf("body:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(out, "nested", "frame.fits")); err != nil {
		t.Fatal(err)
	}

	page, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	html, _ := io.ReadAll(page.Body)
	if page.StatusCode != http.StatusOK || !strings.Contains(string(html), "XISF to FITS") {
		t.Fatalf("page status %d", page.StatusCode)
	}
	if !strings.Contains(string(html), `id="command"`) || !strings.Contains(string(html), "go run ./cmd/xisfits convert") {
		t.Fatal("missing command preview")
	}
	if !strings.Contains(string(html), ">1 core<") {
		t.Fatal("missing 1 core option")
	}
	selected := fmt.Sprintf(`value="%d" selected`, batch.DefaultCores())
	if !strings.Contains(string(html), selected) {
		t.Fatalf("default cores not selected:\n%s", html)
	}
}

func TestBrowseRejectsUnknownKind(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()
	resp, err := http.PostForm(srv.URL+"/browse", url.Values{"kind": {"nope"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCheckAddr(t *testing.T) {
	if err := CheckAddr("127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if err := CheckAddr("0.0.0.0:8080"); err == nil {
		t.Fatal("expected non-loopback address to be rejected")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
