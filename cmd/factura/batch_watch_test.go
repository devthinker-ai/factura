package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBatch_AndWatch(t *testing.T) {
	root := findRoot(t)
	bin := filepath.Join(t.TempDir(), "factura")
	build := exec.Command("go", "build", "-o", bin, "./cmd/factura")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	db := filepath.Join(t.TempDir(), "b.db")
	dir := t.TempDir()
	mustCopy(t, filepath.Join(root, "testdata", "xrechnung-302-cii.xml"), filepath.Join(dir, "valid.xml"))
	mustCopy(t, filepath.Join(root, "testdata", "broken-amounts.xml"), filepath.Join(dir, "broken.xml"))

	out, err := exec.Command(bin, "batch", dir, "--db", db).CombinedOutput()
	if err != nil {
		t.Fatalf("batch: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "ok valid.xml") || !strings.Contains(s, "invalid broken.xml") {
		t.Fatalf("batch out: %s", s)
	}

	out2, err := exec.Command(bin, "batch", dir, "--db", db).CombinedOutput()
	if err != nil {
		t.Fatalf("batch2: %v\n%s", err, out2)
	}
	lines := strings.Split(strings.TrimSpace(string(out2)), "\n")
	dup := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "duplicate ") {
			dup++
		}
	}
	if dup != 2 {
		t.Fatalf("want 2 duplicate lines, got %d: %s", dup, out2)
	}

	// source check via archive list json
	listOut, err := exec.Command(bin, "archive", "list", "--db", db, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("list: %v\n%s", err, listOut)
	}
	if !strings.Contains(string(listOut), "batch:") {
		t.Fatalf("missing source: %s", listOut)
	}

	// watch: drop a third file
	watchDir := t.TempDir()
	mustCopy(t, filepath.Join(root, "testdata", "xrechnung-302-cii.xml"), filepath.Join(watchDir, "a.xml"))
	db2 := filepath.Join(t.TempDir(), "w.db")
	cmd := exec.Command(bin, "watch", watchDir, "--interval", "500ms", "--db", db2)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	mustCopy(t, filepath.Join(root, "testdata", "broken-amounts.xml"), filepath.Join(watchDir, "b.xml"))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "invalid b.xml") || strings.Contains(buf.String(), "ok b.xml") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(buf.String(), "b.xml") {
		_ = cmd.Process.Kill()
		t.Fatalf("watch did not ingest b.xml: %s", buf.String())
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("watch exit: %v\n%s", err, buf.String())
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("watch did not exit on SIGINT")
	}
}

func mustCopy(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func findRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatal("root not found")
	return ""
}
