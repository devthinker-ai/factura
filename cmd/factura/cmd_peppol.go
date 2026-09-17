package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/peppol"
)

func cmdPeppol(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura peppol <send|participants|status|poll> ...")
		return exitUsage
	}
	switch args[0] {
	case "send":
		return peppolSend(args[1:], stdout, stderr)
	case "participants":
		return peppolParticipants(args[1:], stdout, stderr)
	case "status":
		return peppolStatus(args[1:], stdout, stderr)
	case "poll":
		return peppolPoll(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown peppol subcommand: %s\n", args[0])
		return exitUsage
	}
}

func openPeppolEngine(dbPath string, useFake bool) (*archive.Store, *peppol.Engine, error) {
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		return nil, nil, err
	}
	cfg := peppol.ConfigFromEnv()
	if useFake {
		cfg.AP = "fake"
	}
	ap, err := peppol.NewAccessPoint(cfg)
	if err != nil {
		// Fall back to fake when http isn't configured.
		if cfg.AP == "" || cfg.AP == "http" {
			ap = peppol.NewFake()
		} else {
			_ = store.Close()
			return nil, nil, err
		}
	}
	eng := &peppol.Engine{Store: store, AP: ap, SelfID: cfg.SelfID}
	return store, eng, nil
}

func peppolSend(args []string, stdout, stderr io.Writer) int {
	var invoiceID int64
	to, goblPath, dbPath := "", "", ""
	useFake := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--invoice":
			if i+1 >= len(args) {
				return peppolSendUsage(stderr)
			}
			i++
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				fmt.Fprintf(stderr, "bad --invoice: %v\n", err)
				return exitUsage
			}
			invoiceID = n
		case "--to":
			if i+1 >= len(args) {
				return peppolSendUsage(stderr)
			}
			i++
			to = args[i]
		case "--gobl":
			if i+1 >= len(args) {
				return peppolSendUsage(stderr)
			}
			i++
			goblPath = args[i]
		case "--db":
			if i+1 >= len(args) {
				return peppolSendUsage(stderr)
			}
			i++
			dbPath = args[i]
		case "--fake":
			useFake = true
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if to == "" || (invoiceID == 0 && goblPath == "") {
		return peppolSendUsage(stderr)
	}
	store, eng, err := openPeppolEngine(dbPath, useFake || os.Getenv("FACTURA_PEPPOL_AP") == "fake")
	if err != nil {
		fmt.Fprintf(stderr, "peppol: %v\n", err)
		return exitParseError
	}
	defer store.Close()

	req := peppol.SendRequest{InvoiceID: invoiceID, To: to}
	if goblPath != "" {
		data, err := os.ReadFile(goblPath)
		if err != nil {
			fmt.Fprintf(stderr, "read: %v\n", err)
			return exitParseError
		}
		req.GOBL = data
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := eng.Send(ctx, req)
	if err != nil {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		fmt.Fprintf(stderr, "send: %v\n", err)
		switch {
		case errors.Is(err, peppol.ErrValidation):
			return exitInvalid
		case errors.Is(err, peppol.ErrAPRejected):
			return exitInvalid
		case errors.Is(err, peppol.ErrAPUnreachable):
			return exitParseError
		default:
			return exitParseError
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return exitOK
}

func peppolSendUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: factura peppol send --to DE:… (--invoice N | --gobl file.json) [--db path] [--fake]")
	return exitUsage
}

func peppolParticipants(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura peppol participants <add|list> ...")
		return exitUsage
	}
	dbPath := ""
	rest := args[1:]
	switch args[0] {
	case "list":
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--db" && i+1 < len(rest) {
				dbPath = rest[i+1]
			}
		}
		store, err := archive.Open(dbPathOr(dbPath))
		if err != nil {
			fmt.Fprintf(stderr, "db: %v\n", err)
			return exitParseError
		}
		defer store.Close()
		ps, err := store.Participants()
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return exitParseError
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(ps)
		return exitOK
	case "add":
		p := archive.Participant{ServiceType: "buyer-seller"}
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--id":
				if i+1 >= len(rest) {
					return exitUsage
				}
				i++
				p.PeppolID = rest[i]
			case "--name":
				if i+1 >= len(rest) {
					return exitUsage
				}
				i++
				p.Name = rest[i]
			case "--type":
				if i+1 >= len(rest) {
					return exitUsage
				}
				i++
				p.ServiceType = rest[i]
			case "--self":
				p.IsSelf = true
			case "--db":
				if i+1 >= len(rest) {
					return exitUsage
				}
				i++
				dbPath = rest[i]
			}
		}
		if p.PeppolID == "" {
			fmt.Fprintln(stderr, "usage: factura peppol participants add --id DE:… [--name …] [--type buyer|seller|buyer-seller] [--self] [--db path]")
			return exitUsage
		}
		store, err := archive.Open(dbPathOr(dbPath))
		if err != nil {
			fmt.Fprintf(stderr, "db: %v\n", err)
			return exitParseError
		}
		defer store.Close()
		if err := store.AddParticipant(p); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return exitParseError
		}
		fmt.Fprintf(stdout, "added %s\n", p.PeppolID)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown: %s\n", args[0])
		return exitUsage
	}
}

func peppolStatus(args []string, stdout, stderr io.Writer) int {
	dbPath := ""
	useFake := os.Getenv("FACTURA_PEPPOL_AP") == "fake"
	for i := 0; i < len(args); i++ {
		if args[i] == "--db" && i+1 < len(args) {
			dbPath = args[i+1]
		}
		if args[i] == "--fake" {
			useFake = true
		}
	}
	store, eng, err := openPeppolEngine(dbPath, useFake)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitParseError
	}
	defer store.Close()
	counts, _ := store.CountMessagesByStatus("")
	selfID := eng.SelfID
	if selfID == "" {
		if p, err := store.Self(); err == nil {
			selfID = p.PeppolID
		}
	}
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"self_id": selfID,
		"ap_name": eng.AP.Name(),
		"counts":  counts,
	})
	return exitOK
}

func peppolPoll(args []string, stdout, stderr io.Writer) int {
	dbPath := ""
	useFake := os.Getenv("FACTURA_PEPPOL_AP") == "fake"
	for i := 0; i < len(args); i++ {
		if args[i] == "--db" && i+1 < len(args) {
			dbPath = args[i+1]
		}
		if args[i] == "--fake" {
			useFake = true
		}
	}
	store, eng, err := openPeppolEngine(dbPath, useFake)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitParseError
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	results, err := eng.PollAndIngest(ctx, time.Time{})
	if err != nil {
		fmt.Fprintf(stderr, "poll: %v\n", err)
		return exitParseError
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"new": results})
	if len(results) == 0 {
		return exitOK
	}
	for _, r := range results {
		if r.Status == archive.StatusInvalid {
			return exitInvalid
		}
		if r.Status == archive.StatusParseError {
			return exitParseError
		}
	}
	return exitOK
}
