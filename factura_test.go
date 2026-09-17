package factura_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOfflineDeterministic runs core package tests with unroutable proxies.
func TestOfflineDeterministic(t *testing.T) {
	root := findRoot(t)
	cmd := exec.Command("go", "test", "./pkg/parse", "./pkg/validate", "./pkg/model")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"HTTPS_PROXY=http://127.0.0.1:0",
		"HTTP_PROXY=http://127.0.0.1:0",
		"http_proxy=http://127.0.0.1:0",
		"https_proxy=http://127.0.0.1:0",
		"NO_PROXY=",
		"no_proxy=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("offline tests failed: %v\n%s", err, out)
	}
}

// TestBuildAndValidateEmptyDir builds a static binary and runs validate from an empty dir.
func TestBuildAndValidateEmptyDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skip build in short mode")
	}
	root := findRoot(t)
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "factura")
	build := exec.Command("go", "build", "-o", bin, "./cmd/factura")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	empty := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join(root, "testdata", "xrechnung-302-cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	fxPath := filepath.Join(empty, "inv.xml")
	if err := os.WriteFile(fxPath, fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "validate", fxPath)
	cmd.Dir = empty
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate exit: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "valid: true") {
		t.Fatalf("output: %s", out)
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
