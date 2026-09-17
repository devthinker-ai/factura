package api_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDockerSmoke(t *testing.T) {
	// Opt-in only: a full `docker build` of the Go image is too heavy for
	// `go test ./...` on GitHub Actions (and flaky when the daemon is busy).
	if os.Getenv("FACTURA_DOCKER_SMOKE") != "1" {
		t.Skip("set FACTURA_DOCKER_SMOKE=1 to run docker smoke")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not usable: %v\n%s", err, out)
	}
	root := findAPIRoot(t)
	build := exec.Command("docker", "build", "-t", "factura-smoke", ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}
	run := exec.Command("docker", "run", "--rm", "-d",
		"-e", "FACTURA_TOKEN=tok",
		"-p", "18080:8080",
		"--name", "factura-smoke-run",
		"factura-smoke",
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	defer func() { _ = exec.Command("docker", "rm", "-f", "factura-smoke-run").Run() }()

	deadline := time.Now().Add(45 * time.Second)
	var last []byte
	var err error
	for time.Now().Before(deadline) {
		curl := exec.Command("curl", "-sf", "http://127.0.0.1:18080/health")
		last, err = curl.CombinedOutput()
		if err == nil && len(last) > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("health not ready: %v %s", err, last)
}

func findAPIRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatal("go.mod not found")
	return ""
}
