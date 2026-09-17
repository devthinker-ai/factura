package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/peppol"
)

func cmdServe(args []string, stdout, stderr io.Writer) int {
	addr := ":8080"
	if v := os.Getenv("FACTURA_ADDR"); v != "" {
		addr = v
	}
	dbPath := ""
	peppolFake := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--addr":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura serve [--addr :8080] [--db path] [--peppol-fake]")
				return exitUsage
			}
			i++
			addr = args[i]
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura serve [--addr :8080] [--db path] [--peppol-fake]")
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "--peppol-fake":
			peppolFake = true
		case "-h", "--help":
			fmt.Fprint(stdout, `factura serve — HTTP API over the Phase 1 engine

Usage:
  factura serve [--addr :8080] [--db path] [--peppol-fake]

Env:
  FACTURA_ADDR        listen address (overrides --addr default :8080)
  FACTURA_DB          SQLite path (same as --db)
  FACTURA_TOKEN       if set, require Authorization: Bearer <token> (except GET /health)
  FACTURA_RATE_LIMIT  req/min when auth on (default 60)
  FACTURA_PEPPOL_*    AP / SELF_ID / POLL — see AGENTS.md

Recommend binding to 127.0.0.1 behind a reverse proxy for TLS.
CORS is not enabled (same-origin UI at /app/).
`)
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}

	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()

	srv := api.New(store)
	srv.Version = version

	cfg := peppol.ConfigFromEnv()
	if peppolFake || cfg.AP == "fake" {
		cfg.AP = "fake"
	}
	ap, err := peppol.NewAccessPoint(cfg)
	if err != nil {
		// No real AP configured (no URL) → run on the in-memory Fake so the
		// engine is shippable without a Peppol vendor (GTM §8: the Fake keeps
		// everything testable/running offline). Configured AP (URL set) →
		// hard error, because a typo in the AP address must not be silently
		// swallowed by the fake.
		if cfg.APURL == "" {
			fmt.Fprintf(stdout, "peppol: no AP configured (FACTURA_PEPPOL_AP_URL empty) — using fake\n")
			ap = peppol.NewFake()
		} else {
			fmt.Fprintf(stderr, "peppol: %v\n", err)
			return exitParseError
		}
	}
	eng := &peppol.Engine{Store: store, AP: ap, SelfID: cfg.SelfID}
	srv.SetPeppol(eng)

	fmt.Fprintf(stdout, "factura serve listening on %s (db=%s auth=%v version=%s peppol=%s)\n",
		addr, dbPathOr(dbPath), srv.Token != "", srv.Version, ap.Name())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv.StartPeppolPoller(ctx)
	defer srv.StopPeppolPoller()
	if err := api.ListenAndServe(ctx, addr, srv.Handler()); err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return exitParseError
	}
	return exitOK
}

func cmdBatch(args []string, stdout, stderr io.Writer) int {
	dir, source, dbPath, err := parseDirFlags(args, "batch")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if dir == "" {
		fmt.Fprintln(stderr, "usage: factura batch <dir> [--source tag] [--db path]")
		return exitUsage
	}
	if source == "" {
		source = "batch:" + dir
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitInvalid
	}
	defer store.Close()

	if code := ingestDir(store, dir, source, stdout, stderr, nil); code != exitOK {
		return code
	}
	return exitOK
}

func cmdWatch(args []string, stdout, stderr io.Writer) int {
	dir, source, dbPath, err := parseDirFlags(args, "watch")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	interval := 10 * time.Second
	for i := 0; i < len(args); i++ {
		if args[i] == "--interval" {
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura watch <dir> [--interval 10s] [--source tag] [--db path]")
				return exitUsage
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "bad --interval: %v\n", err)
				return exitUsage
			}
			interval = d
			break
		}
	}
	if dir == "" {
		fmt.Fprintln(stderr, "usage: factura watch <dir> [--interval 10s] [--source tag] [--db path]")
		return exitUsage
	}
	if source == "" {
		source = "watch:" + dir
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitInvalid
	}
	defer store.Close()

	// WHY polling (not fsnotify): keeps deps flat in v1. Upgrade path: add
	// fsnotify watcher and keep this map as a fallback for NFS/network shares.
	seen := map[string]fileStamp{}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Immediate first pass.
	_ = ingestDir(store, dir, source, stdout, stderr, seen)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return exitOK
		case <-ticker.C:
			_ = ingestDir(store, dir, source, stdout, stderr, seen)
		}
	}
}

type fileStamp struct {
	size  int64
	mtime int64
}

func ingestDir(store *archive.Store, dir, source string, stdout, stderr io.Writer, seen map[string]fileStamp) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "readdir: %v\n", err)
		return exitInvalid
	}
	ioFail := false
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			fmt.Fprintf(stderr, "stat %s: %v\n", e.Name(), err)
			ioFail = true
			continue
		}
		if seen != nil {
			st := fileStamp{size: info.Size(), mtime: info.ModTime().UnixNano()}
			if prev, ok := seen[e.Name()]; ok && prev == st {
				continue
			}
			seen[e.Name()] = st
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "read %s: %v\n", e.Name(), err)
			ioFail = true
			continue
		}
		// WHY idHint = source:filename: folder files that share vendor/number/date
		// (e.g. golden + broken mutations) must still archive separately; re-scan
		// of the same path then hits ErrDuplicate as the retry contract.
		idHint := source + ":" + e.Name()
		row, err := store.IngestWithSource(data, idHint, source)
		if err != nil {
			if errorsIsDuplicate(err) {
				fmt.Fprintf(stdout, "duplicate %s id=%d\n", e.Name(), row.ID)
				continue
			}
			fmt.Fprintf(stderr, "ingest %s: %v\n", e.Name(), err)
			ioFail = true
			continue
		}
		switch row.Status {
		case archive.StatusValid:
			fmt.Fprintf(stdout, "ok %s id=%d\n", e.Name(), row.ID)
		case archive.StatusInvalid:
			fmt.Fprintf(stdout, "invalid %s id=%d\n", e.Name(), row.ID)
		default:
			fmt.Fprintf(stdout, "parse_error %s id=%d\n", e.Name(), row.ID)
		}
	}
	if ioFail {
		return exitInvalid
	}
	return exitOK
}

func errorsIsDuplicate(err error) bool {
	return errors.Is(err, archive.ErrDuplicate)
}

func parseDirFlags(args []string, cmd string) (dir, source, dbPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("usage: factura %s <dir> [--source tag] [--db path]", cmd)
			}
			i++
			source = args[i]
		case "--db":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("usage: factura %s <dir> [--source tag] [--db path]", cmd)
			}
			i++
			dbPath = args[i]
		case "--interval":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--interval needs a value")
			}
			i++ // consumed by cmdWatch
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", fmt.Errorf("unknown flag: %s", args[i])
			}
			dir = args[i]
		}
	}
	return dir, source, dbPath, nil
}
