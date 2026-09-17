package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/license"
)

const exitUnlicensed = 4

const defaultPrivKeyPath = ".factura-license-priv.pem"

func cmdLicense(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura license check|mint|remove …")
		return exitUsage
	}
	switch args[0] {
	case "check":
		return cmdLicenseCheck(args[1:], stdout, stderr)
	case "mint":
		return cmdLicenseMint(args[1:], stdout, stderr)
	case "remove":
		return cmdLicenseRemove(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown license subcommand: %s\n", args[0])
		return exitUsage
	}
}

func cmdLicenseCheck(args []string, stdout, stderr io.Writer) int {
	key := ""
	dbPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--license":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura license check [--license <key>] [--db path]")
				return exitUsage
			}
			i++
			key = args[i]
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura license check [--license <key>] [--db path]")
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprint(stdout, `factura license check — resolve and print license caps as JSON

Usage:
  factura license check [--license <key>] [--db path]

Precedence: --license > FACTURA_LICENSE_KEY > settings.license_key in --db.
Exit: 0 licensed, 4 unlicensed/invalid/expired.
`)
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}

	var settings license.SettingsReader
	if dbPath != "" || key == "" {
		store, err := archive.Open(dbPathOr(dbPath))
		if err != nil {
			fmt.Fprintf(stderr, "db: %v\n", err)
			return exitParseError
		}
		defer store.Close()
		settings = store
	}
	caps := license.Resolve(nil, key, settings)
	body := map[string]any{
		"plan":     caps.Plan,
		"licensed": caps.Licensed,
		"subject":  caps.Subject,
		"expires":  "",
		"caps": map[string]any{
			"max_companies": caps.MaxCompanies,
			"max_api_keys":  caps.MaxAPIKeys,
			"max_clients":   caps.MaxClients,
			"peppol":        caps.Peppol,
		},
	}
	if !caps.ExpiresAt.IsZero() {
		body["expires"] = caps.ExpiresAt.UTC().Format(time.RFC3339)
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
	if !caps.Licensed {
		return exitUnlicensed
	}
	return exitOK
}

func cmdLicenseMint(args []string, stdout, stderr io.Writer) int {
	plan := ""
	subject := ""
	expires := ""
	companies, apiKeys, clients := 0, 0, 0
	keyPath := filepath.Join(homeDir(), defaultPrivKeyPath)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			plan = args[i]
		case "--subject":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			subject = args[i]
		case "--expires":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			expires = args[i]
		case "--companies":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			companies, _ = strconv.Atoi(args[i])
		case "--api-keys":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			apiKeys, _ = strconv.Atoi(args[i])
		case "--clients":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			clients, _ = strconv.Atoi(args[i])
		case "--key":
			if i+1 >= len(args) {
				return mintUsage(stderr)
			}
			i++
			keyPath = args[i]
		case "-h", "--help":
			fmt.Fprint(stdout, `factura license mint — sign an RS256 license JWT

Usage:
  factura license mint --plan solo|pro|kanzlei --subject <email> --expires YYYY-MM-DD
    [--companies N] [--api-keys N] [--clients N] [--key ~/.factura-license-priv.pem]

Prints the JWT alone on stdout (scriptable). Private key never ships in the binary.
`)
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return mintUsage(stderr)
		}
	}
	plan = strings.ToLower(strings.TrimSpace(plan))
	if plan != license.PlanSolo && plan != license.PlanPro && plan != license.PlanKanzlei {
		fmt.Fprintln(stderr, "mint: --plan must be solo|pro|kanzlei")
		return exitUsage
	}
	if strings.TrimSpace(subject) == "" {
		fmt.Fprintln(stderr, "mint: --subject required")
		return exitUsage
	}
	if expires == "" {
		fmt.Fprintln(stderr, "mint: --expires YYYY-MM-DD required")
		return exitUsage
	}
	expDay, err := time.Parse("2006-01-02", expires)
	if err != nil {
		fmt.Fprintf(stderr, "mint: bad --expires: %v\n", err)
		return exitUsage
	}
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(stderr, "mint: private key %s: %v\n", keyPath, err)
		return exitParseError
	}
	priv, err := license.ParseRSAPrivateKey(pemBytes)
	if err != nil {
		fmt.Fprintf(stderr, "mint: parse private key: %v\n", err)
		return exitParseError
	}
	claims := license.Claims{
		Plan:         plan,
		MaxCompanies: companies,
		MaxAPIKeys:   apiKeys,
		MaxClients:   clients,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(expDay.UTC().Add(24*time.Hour - time.Second)),
		},
	}
	tok, err := license.Sign(priv, claims)
	if err != nil {
		fmt.Fprintf(stderr, "mint: sign: %v\n", err)
		return exitParseError
	}
	fmt.Fprintln(stdout, tok)
	return exitOK
}

func mintUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: factura license mint --plan solo|pro|kanzlei --subject <email> --expires YYYY-MM-DD […]")
	return exitUsage
}

func cmdLicenseRemove(args []string, stdout, stderr io.Writer) int {
	dbPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura license remove [--db path]")
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprint(stdout, "factura license remove — delete persisted license_key from settings\n")
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
	if err := store.DeleteSetting("license_key"); err != nil {
		if err == archive.ErrNotFound {
			fmt.Fprintln(stderr, "no license_key persisted")
			return exitParseError
		}
		fmt.Fprintf(stderr, "remove: %v\n", err)
		return exitParseError
	}
	fmt.Fprintln(stdout, "license_key removed")
	return exitOK
}

func cmdAPIKeys(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura apikeys create|list|delete …")
		return exitUsage
	}
	switch args[0] {
	case "create":
		return cmdAPIKeysCreate(args[1:], stdout, stderr)
	case "list":
		return cmdAPIKeysList(args[1:], stdout, stderr)
	case "delete":
		return cmdAPIKeysDelete(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown apikeys subcommand: %s\n", args[0])
		return exitUsage
	}
}

func cmdAPIKeysCreate(args []string, stdout, stderr io.Writer) int {
	name, dbPath := "", ""
	rpm, monthly := 0, 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return apikeysUsage(stderr)
			}
			i++
			name = args[i]
		case "--rpm":
			if i+1 >= len(args) {
				return apikeysUsage(stderr)
			}
			i++
			rpm, _ = strconv.Atoi(args[i])
		case "--monthly":
			if i+1 >= len(args) {
				return apikeysUsage(stderr)
			}
			i++
			monthly, _ = strconv.Atoi(args[i])
		case "--db":
			if i+1 >= len(args) {
				return apikeysUsage(stderr)
			}
			i++
			dbPath = args[i]
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return apikeysUsage(stderr)
		}
	}
	if strings.TrimSpace(name) == "" {
		return apikeysUsage(stderr)
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	k, err := store.CreateKey(name, rpm, monthly)
	if err != nil {
		fmt.Fprintf(stderr, "create: %v\n", err)
		return exitParseError
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"key":              k.ID,
		"name":             k.Name,
		"rpm":              k.RPM,
		"monthly_invoices": k.MonthlyInvoices,
	})
	return exitOK
}

func cmdAPIKeysList(args []string, stdout, stderr io.Writer) int {
	dbPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura apikeys list [--db path]")
				return exitUsage
			}
			i++
			dbPath = args[i]
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
	keys, err := store.ListKeys()
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return exitParseError
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{
			"id":                  k.ID,
			"name":                k.Name,
			"rpm":                 k.RPM,
			"monthly_invoices":    k.MonthlyInvoices,
			"invoices_this_month": k.InvoicesThisMonth,
			"enabled":             k.Enabled,
			"created_at":          k.CreatedAt,
		})
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"keys": out})
	return exitOK
}

func cmdAPIKeysDelete(args []string, stdout, stderr io.Writer) int {
	id, dbPath := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura apikeys delete --id <factura_…> [--db path]")
				return exitUsage
			}
			i++
			id = args[i]
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		default:
			if id == "" && !strings.HasPrefix(args[i], "-") {
				id = args[i]
				continue
			}
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if id == "" {
		fmt.Fprintln(stderr, "usage: factura apikeys delete --id <factura_…> [--db path]")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	if err := store.DeleteKey(id); err != nil {
		fmt.Fprintf(stderr, "delete: %v\n", err)
		return exitParseError
	}
	return exitOK
}

func apikeysUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: factura apikeys create --name <n> [--rpm N] [--monthly N] [--db path]")
	return exitUsage
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}
