// Command factura — Phase 1 CLI: parse, validate, generate, archive.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/generate"
	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// Exit codes — the whole contract for cron/watch integrations.
const (
	exitOK         = 0
	exitInvalid    = 1
	exitParseError = 2
	exitUsage      = 3
	// exitUnlicensed = 4 — see cmd_license.go
)

// version is set at link time: -ldflags "-X main.version=$(VERSION)".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stderr)
		return exitUsage
	}
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "parse":
		return cmdParse(rest, stdout, stderr)
	case "validate":
		return cmdValidate(rest, stdout, stderr)
	case "generate":
		return cmdGenerate(rest, stdout, stderr)
	case "archive":
		return cmdArchive(rest, stdout, stderr)
	case "serve":
		return cmdServe(rest, stdout, stderr)
	case "batch":
		return cmdBatch(rest, stdout, stderr)
	case "watch":
		return cmdWatch(rest, stdout, stderr)
	case "peppol":
		return cmdPeppol(rest, stdout, stderr)
	case "license":
		return cmdLicense(rest, stdout, stderr)
	case "apikeys":
		return cmdAPIKeys(rest, stdout, stderr)
	case "user":
		return cmdUser(rest, stdout, stderr)
	case "verify":
		return cmdVerify(rest, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", cmd)
		printUsage(stderr)
		return exitUsage
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `factura — self-hosted EU e-invoicing engine

Usage:
  factura parse <file> [--json]
  factura validate <file> [--json]
  factura generate --from <invoice.json> --outdir <dir> [--json]
  factura archive add <file> [--id ext-id] [--db path] [--json]
  factura archive list [--status s] [--vendor q] [--since d] [--until d] [--db path] [--json]
  factura archive show <id> [--db path] [--json]
  factura archive original <id> [-o file] [--db path]
  factura archive export [--status s] [--vendor q] [--since d] [--until d] [--out file] [--db path]
  factura serve [--addr :8080] [--db path] [--peppol-fake]
  factura batch <dir> [--source tag] [--db path]
  factura watch <dir> [--interval 10s] [--source tag] [--db path]
  factura peppol send --to DE:… (--invoice N | --gobl file.json) [--db path] [--fake]
  factura peppol participants add --id DE:… [--name …] [--type …] [--self] [--db path]
  factura peppol participants list [--db path]
  factura peppol status [--db path] [--fake]
  factura peppol poll [--db path] [--fake]
  factura license check [--license <key>] [--db path]
  factura license mint --plan solo|pro|kanzlei --subject <email> --expires YYYY-MM-DD […]
  factura license remove [--db path]
  factura apikeys create --name <n> [--rpm N] [--monthly N] [--db path]
  factura apikeys list|delete [--db path]
  factura user add --email <e> [--name <n>] [--role admin|editor|viewer] [--password <p>|--password-stdin] [--db path]
  factura user list|remove|password|role [--db path]
  factura verify [--db path] [--json]

Exit codes: 0 valid/ok, 1 invalid, 2 parse error, 3 usage error, 4 unlicensed.
DB default: ./factura.db (override with FACTURA_DB or --db).
Serve: FACTURA_TOKEN enables bearer auth; FACTURA_RATE_LIMIT sets req/min when auth on.
  FACTURA_LICENSE_KEY sets an offline RS256 license (or paste via POST /license).
  FACTURA_SESSION_SECRET overrides the HS256 session signing secret (else auto-persisted).
Peppol: FACTURA_PEPPOL_AP=fake|http, FACTURA_PEPPOL_AP_URL, FACTURA_PEPPOL_AP_KEY,
  FACTURA_PEPPOL_SELF_ID, FACTURA_PEPPOL_CALLBACK_URL, FACTURA_PEPPOL_POLL=1.
`)
}

func cmdParse(args []string, stdout, stderr io.Writer) int {
	asJSON, file, err := takeFileFlag(args)
	if err != nil || file == "" {
		fmt.Fprintln(stderr, "usage: factura parse <file> [--json]")
		return exitUsage
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "read: %v\n", err)
		return exitParseError
	}
	inv, format, err := parse.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitParseError
	}
	goblJSON, err := inv.JSON()
	if err != nil {
		fmt.Fprintf(stderr, "json: %v\n", err)
		return exitParseError
	}
	if asJSON {
		out := map[string]any{"format": format, "document": json.RawMessage(goblJSON)}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return exitOK
	}
	fmt.Fprintf(stdout, "format: %s\n", format)
	fmt.Fprintln(stdout, string(goblJSON))
	return exitOK
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	asJSON, file, err := takeFileFlag(args)
	if err != nil || file == "" {
		fmt.Fprintln(stderr, "usage: factura validate <file> [--json]")
		return exitUsage
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "read: %v\n", err)
		return exitParseError
	}
	// Detect obvious non-invoice before validation for exit 2.
	if parse.DetectFormat(data) == parse.FormatUnknown {
		var pe *parse.Error
		_, _, err := parse.Parse(data)
		if errors.As(err, &pe) || err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return exitParseError
		}
	}
	rep := validate.ValidateBytes(data)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printReport(stdout, rep)
	}
	if rep.Valid {
		return exitOK
	}
	// Schema-level XML-PARSE → exit 2; business rule failures → exit 1.
	if rep.Level == "schema" {
		for _, v := range rep.Violations {
			if v.RuleID == "XML-PARSE" {
				return exitParseError
			}
		}
	}
	return exitInvalid
}

func cmdGenerate(args []string, stdout, stderr io.Writer) int {
	from, outdir := "", ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura generate --from <invoice.json> --outdir <dir>")
				return exitUsage
			}
			i++
			from = args[i]
		case "--outdir":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura generate --from <invoice.json> --outdir <dir>")
				return exitUsage
			}
			i++
			outdir = args[i]
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if from == "" || outdir == "" {
		fmt.Fprintln(stderr, "usage: factura generate --from <invoice.json> --outdir <dir>")
		return exitUsage
	}
	data, err := os.ReadFile(from)
	if err != nil {
		fmt.Fprintf(stderr, "read: %v\n", err)
		return exitParseError
	}
	inv, err := model.FromJSON(data)
	if err != nil {
		fmt.Fprintf(stderr, "parse gobl: %v\n", err)
		return exitParseError
	}
	xr, zf, err := generate.Generate(*inv, outdir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitInvalid
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(map[string]string{
			"xrechnung": xr,
			"zugferd":   zf,
		})
	} else {
		fmt.Fprintf(stdout, "xrechnung: %s\nzugferd: %s\n", xr, zf)
	}
	return exitOK
}

func cmdArchive(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura archive <add|list|show|original|export> ...")
		return exitUsage
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "add":
		return archiveAdd(rest, stdout, stderr)
	case "list":
		return archiveList(rest, stdout, stderr)
	case "show":
		return archiveShow(rest, stdout, stderr)
	case "original":
		return archiveOriginal(rest, stdout, stderr)
	case "export":
		return archiveExport(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown archive subcommand: %s\n", sub)
		return exitUsage
	}
}

func archiveAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	idHint := ""
	dbPath := ""
	var file string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--id":
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			i++
			idHint = args[i]
		case "--db":
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			i++
			dbPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
				return exitUsage
			}
			file = args[i]
		}
	}
	if file == "" {
		return usageArchive(stderr)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "read: %v\n", err)
		return exitParseError
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	row, err := store.Ingest(data, idHint)
	if err != nil {
		fmt.Fprintf(stderr, "ingest: %v\n", err)
		return exitParseError
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(row)
	} else {
		fmt.Fprintf(stdout, "id=%d status=%s format=%s vendor=%q number=%q total=%.2f\n",
			row.ID, row.Status, row.Format, row.VendorName, row.InvoiceNumber, row.Total)
	}
	switch row.Status {
	case archive.StatusValid:
		return exitOK
	case archive.StatusInvalid:
		return exitInvalid
	default:
		return exitParseError
	}
}

func archiveList(args []string, stdout, stderr io.Writer) int {
	f, dbPath, asJSON, err := parseFilterFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	rows, err := store.List(f)
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return exitParseError
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
		return exitOK
	}
	for _, r := range rows {
		fmt.Fprintf(stdout, "%d\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
			r.ID, r.Status, r.Format, r.VendorName, r.InvoiceNumber, r.Total, r.InvoiceDate)
	}
	return exitOK
}

func archiveShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	dbPath := ""
	var idStr string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--db":
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			i++
			dbPath = args[i]
		default:
			idStr = args[i]
		}
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || idStr == "" {
		fmt.Fprintln(stderr, "usage: factura archive show <id>")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	row, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitParseError
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(row)
		return exitOK
	}
	fmt.Fprintf(stdout, "id: %d\nexternal_id: %s\nformat: %s\nstatus: %s\nvendor: %s\nbuyer: %s\nnumber: %s\ndate: %s\ntotal: %.2f\nvat: %.2f\ncurrency: %s\ncreated: %s\n",
		row.ID, row.ExternalID, row.Format, row.Status, row.VendorName, row.BuyerName,
		row.InvoiceNumber, row.InvoiceDate, row.Total, row.VATAmount, row.Currency, row.CreatedAt)
	fmt.Fprintln(stdout, "--- report ---")
	fmt.Fprintln(stdout, string(row.Report))
	fmt.Fprintln(stdout, "--- gobl ---")
	fmt.Fprintln(stdout, string(row.DocGOBL))
	return exitOK
}

func archiveOriginal(args []string, stdout, stderr io.Writer) int {
	dbPath, outPath, idStr := "", "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			i++
			outPath = args[i]
		case "--db":
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			i++
			dbPath = args[i]
		default:
			idStr = args[i]
		}
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || idStr == "" {
		fmt.Fprintln(stderr, "usage: factura archive original <id> [-o file]")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	blob, err := store.Original(id)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitParseError
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, blob, 0o644); err != nil {
			fmt.Fprintf(stderr, "write: %v\n", err)
			return exitParseError
		}
		fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", outPath, len(blob))
		return exitOK
	}
	_, _ = stdout.Write(blob)
	return exitOK
}

func archiveExport(args []string, stdout, stderr io.Writer) int {
	f, dbPath, _, err := parseFilterFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	outPath := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--out" || args[i] == "-o" {
			if i+1 >= len(args) {
				return usageArchive(stderr)
			}
			outPath = args[i+1]
			break
		}
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	var w io.Writer = stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintf(stderr, "create: %v\n", err)
			return exitParseError
		}
		defer f.Close()
		w = f
	}
	if err := store.Export(f, w); err != nil {
		fmt.Fprintf(stderr, "export: %v\n", err)
		return exitParseError
	}
	return exitOK
}

func parseFilterFlags(args []string) (archive.Filter, string, bool, error) {
	var f archive.Filter
	dbPath := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--status needs a value")
			}
			i++
			f.Status = args[i]
		case "--vendor":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--vendor needs a value")
			}
			i++
			f.Vendor = args[i]
		case "--since":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--since needs a value")
			}
			i++
			f.Since = args[i]
		case "--until":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--until needs a value")
			}
			i++
			f.Until = args[i]
		case "--db":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--db needs a value")
			}
			i++
			dbPath = args[i]
		case "--json":
			asJSON = true
		case "--out", "-o":
			if i+1 >= len(args) {
				return f, "", false, fmt.Errorf("--out needs a value")
			}
			i++ // skip value; export reads it separately
		default:
			if strings.HasPrefix(args[i], "-") {
				return f, "", false, fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}
	return f, dbPath, asJSON, nil
}

func takeFileFlag(args []string) (asJSON bool, file string, err error) {
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				return false, "", fmt.Errorf("unknown flag: %s", a)
			}
			file = a
		}
	}
	return asJSON, file, nil
}

func dbPathOr(p string) string {
	if p != "" {
		return p
	}
	return archive.DefaultDBPath()
}

func usageArchive(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: factura archive <add|list|show|original|export> ...")
	return exitUsage
}

func printReport(w io.Writer, rep validate.Report) {
	fmt.Fprintf(w, "valid: %v\nformat: %s\nlevel: %s\nsummary: %s\n", rep.Valid, rep.Format, rep.Level, rep.Summary)
	if len(rep.Violations) == 0 {
		return
	}
	fmt.Fprintln(w, "violations:")
	for _, v := range rep.Violations {
		line := fmt.Sprintf("  %s\t%s", v.RuleID, v.Human)
		if v.Value != "" {
			line += "\t" + v.Value
		}
		fmt.Fprintln(w, line)
	}
}
