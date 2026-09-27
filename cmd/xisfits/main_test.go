package main

import "testing"

func TestConvertRequiresDirectories(t *testing.T) {
	if code := runConvert(nil); code == 0 {
		t.Fatal("missing flags should fail")
	}
	if code := runConvert([]string{"--input", t.TempDir()}); code == 0 {
		t.Fatal("missing --output should fail")
	}
	if code := runConvert([]string{"--output", t.TempDir()}); code == 0 {
		t.Fatal("missing --input should fail")
	}
}

func TestGUIRejectsPublicAddr(t *testing.T) {
	if code := runGUI([]string{"--addr", "0.0.0.0:9"}); code == 0 {
		t.Fatal("public address should fail")
	}
}

func TestGUIListenAddr(t *testing.T) {
	addr, err := guiListenAddr("", 8080, false, false)
	if err != nil || addr != "127.0.0.1:8080" {
		t.Fatalf("default: %q %v", addr, err)
	}
	addr, err = guiListenAddr("", 9090, false, true)
	if err != nil || addr != "127.0.0.1:9090" {
		t.Fatalf("port: %q %v", addr, err)
	}
	if _, err := guiListenAddr("", 0, false, true); err == nil {
		t.Fatal("port 0 should fail")
	}
	if _, err := guiListenAddr("", 65536, false, true); err == nil {
		t.Fatal("port 65536 should fail")
	}
	if _, err := guiListenAddr("127.0.0.1:9", 9090, true, true); err == nil {
		t.Fatal("both flags should fail")
	}
	addr, err = guiListenAddr("127.0.0.1:9", 8080, true, false)
	if err != nil || addr != "127.0.0.1:9" {
		t.Fatalf("addr: %q %v", addr, err)
	}
}

func TestGUIRejectsBadPort(t *testing.T) {
	if code := runGUI([]string{"--port", "0"}); code == 0 {
		t.Fatal("port 0 should fail")
	}
	if code := runGUI([]string{"--port", "9090", "--addr", "127.0.0.1:1"}); code == 0 {
		t.Fatal("both flags should fail")
	}
}

func TestHelp(t *testing.T) {
	if code := run([]string{"help"}); code != 0 {
		t.Fatal(code)
	}
	if code := run(nil); code == 0 {
		t.Fatal("no args should fail")
	}
}
