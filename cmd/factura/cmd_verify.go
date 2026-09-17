package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/devthinker-ai/factura/pkg/archive"
)

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	dbPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura verify [--db path] [--json]")
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: factura verify [--db path] [--json]")
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			fmt.Fprintln(stderr, "usage: factura verify [--db path] [--json]")
			return exitUsage
		}
	}
	if dbPath == "" {
		dbPath = archive.DefaultDBPath()
	}
	store, err := archive.Open(dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify: open db: %v\n", err)
		return exitParseError
	}
	defer store.Close()

	rep, err := store.VerifyChain()
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitParseError
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else if rep.OK {
		fmt.Fprintf(stdout, "OK — %d links, head %s\n", rep.LinkCount, rep.Head)
	} else {
		fmt.Fprintf(stdout, "BROKEN — first broken invoice_id=%d expected=%s actual=%s\n",
			rep.FirstBrokenID, rep.ExpectedHash, rep.ActualHash)
	}
	if !rep.OK {
		return exitInvalid
	}
	return exitOK
}
