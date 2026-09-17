package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/session"
)

func cmdUser(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: factura user add|list|remove|password|role …")
		return exitUsage
	}
	switch args[0] {
	case "add":
		return cmdUserAdd(args[1:], stdout, stderr)
	case "list":
		return cmdUserList(args[1:], stdout, stderr)
	case "remove":
		return cmdUserRemove(args[1:], stdout, stderr)
	case "password":
		return cmdUserPassword(args[1:], stdout, stderr)
	case "role":
		return cmdUserRole(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown user subcommand: %s\n", args[0])
		return exitUsage
	}
}

func cmdUserAdd(args []string, stdout, stderr io.Writer) int {
	email, name, role, password, dbPath := "", "", "", "", ""
	passwordStdin := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--email":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "usage: factura user add --email <e> [--name <n>] [--role admin|editor|viewer] [--password <p>|--password-stdin] [--db path]")
				return exitUsage
			}
			i++
			email = args[i]
		case "--name":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			name = args[i]
		case "--role":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			role = args[i]
		case "--password":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			password = args[i]
		case "--password-stdin":
			passwordStdin = true
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprint(stdout, `factura user add — create a console user

Usage:
  factura user add --email <e> [--name <n>] [--role admin|editor|viewer]
    [--password <p>|--password-stdin] [--db path]

First user in an empty DB defaults to role admin.
Prefer --password-stdin (or TTY prompt) over --password.
`)
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if email == "" {
		fmt.Fprintln(stderr, "usage: factura user add --email <e> …")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()

	n, err := store.CountUsers()
	if err != nil {
		fmt.Fprintf(stderr, "count: %v\n", err)
		return exitParseError
	}
	if role == "" {
		if n == 0 {
			role = archive.RoleAdmin
		} else {
			role = archive.RoleViewer
		}
	}
	if !archive.ValidRole(role) {
		fmt.Fprintf(stderr, "invalid role: %s\n", role)
		return exitInvalid
	}
	pw, code := resolvePassword(password, passwordStdin, stderr)
	if code != exitOK {
		return code
	}
	if len(pw) < 10 {
		fmt.Fprintln(stderr, "password too short (min 10)")
		return exitInvalid
	}
	hash, err := session.FormatHash(pw)
	if err != nil {
		fmt.Fprintf(stderr, "hash: %v\n", err)
		return exitParseError
	}
	u, err := store.AddUser(email, name, role, hash)
	if err == archive.ErrDuplicateEmail {
		fmt.Fprintf(stderr, "email already exists: %s\n", email)
		return exitInvalid
	}
	if err != nil {
		fmt.Fprintf(stderr, "add: %v\n", err)
		return exitParseError
	}
	fmt.Fprintf(stdout, "created user %s (%s) role=%s\n", u.Email, u.ID, u.Role)
	return exitOK
}

func cmdUserList(args []string, stdout, stderr io.Writer) int {
	dbPath := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: factura user list [--db path] [--json]")
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
	users, err := store.ListUsers()
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return exitParseError
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{"users": users})
		return exitOK
	}
	for _, u := range users {
		totp := "off"
		if u.TOTPEnabled {
			totp = "on"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t2fa=%s\t%s\n", u.Email, u.Name, u.Role, totp, u.CreatedAt)
	}
	return exitOK
}

func cmdUserRemove(args []string, stdout, stderr io.Writer) int {
	email, dbPath := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--email":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			email = args[i]
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: factura user remove --email <e> [--db path]")
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if email == "" {
		fmt.Fprintln(stderr, "usage: factura user remove --email <e> [--db path]")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	u, err := store.UserByEmail(email)
	if err == archive.ErrNotFound {
		fmt.Fprintf(stderr, "unknown email: %s\n", email)
		return exitInvalid
	}
	if err != nil {
		fmt.Fprintf(stderr, "lookup: %v\n", err)
		return exitParseError
	}
	if err := store.RemoveUser(u.ID); err != nil {
		fmt.Fprintf(stderr, "remove: %v\n", err)
		return exitParseError
	}
	fmt.Fprintf(stdout, "removed %s\n", u.Email)
	return exitOK
}

func cmdUserPassword(args []string, stdout, stderr io.Writer) int {
	email, password, dbPath := "", "", ""
	passwordStdin := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--email":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			email = args[i]
		case "--password":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			password = args[i]
		case "--password-stdin":
			passwordStdin = true
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: factura user password --email <e> [--password <p>|--password-stdin] [--db path]")
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if email == "" {
		fmt.Fprintln(stderr, "usage: factura user password --email <e> …")
		return exitUsage
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	u, err := store.UserByEmail(email)
	if err == archive.ErrNotFound {
		fmt.Fprintf(stderr, "unknown email: %s\n", email)
		return exitInvalid
	}
	if err != nil {
		fmt.Fprintf(stderr, "lookup: %v\n", err)
		return exitParseError
	}
	pw, code := resolvePassword(password, passwordStdin, stderr)
	if code != exitOK {
		return code
	}
	if len(pw) < 10 {
		fmt.Fprintln(stderr, "password too short (min 10)")
		return exitInvalid
	}
	hash, err := session.FormatHash(pw)
	if err != nil {
		fmt.Fprintf(stderr, "hash: %v\n", err)
		return exitParseError
	}
	if err := store.SetPassword(u.ID, hash); err != nil {
		fmt.Fprintf(stderr, "set: %v\n", err)
		return exitParseError
	}
	fmt.Fprintf(stdout, "password updated for %s\n", u.Email)
	return exitOK
}

func cmdUserRole(args []string, stdout, stderr io.Writer) int {
	email, role, dbPath := "", "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--email":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			email = args[i]
		case "--role":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			role = args[i]
		case "--db":
			if i+1 >= len(args) {
				return exitUsage
			}
			i++
			dbPath = args[i]
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: factura user role --email <e> --role <r> [--db path]")
			return exitOK
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n", args[i])
			return exitUsage
		}
	}
	if email == "" || role == "" {
		fmt.Fprintln(stderr, "usage: factura user role --email <e> --role <r> [--db path]")
		return exitUsage
	}
	if !archive.ValidRole(role) {
		fmt.Fprintf(stderr, "invalid role: %s\n", role)
		return exitInvalid
	}
	store, err := archive.Open(dbPathOr(dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "db: %v\n", err)
		return exitParseError
	}
	defer store.Close()
	u, err := store.UserByEmail(email)
	if err == archive.ErrNotFound {
		fmt.Fprintf(stderr, "unknown email: %s\n", email)
		return exitInvalid
	}
	if err != nil {
		fmt.Fprintf(stderr, "lookup: %v\n", err)
		return exitParseError
	}
	if err := store.SetRole(u.ID, role); err != nil {
		fmt.Fprintf(stderr, "set: %v\n", err)
		return exitParseError
	}
	fmt.Fprintf(stdout, "role updated for %s → %s\n", u.Email, role)
	return exitOK
}

func resolvePassword(flag string, fromStdin bool, stderr io.Writer) (string, int) {
	if fromStdin {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(stderr, "read password: %v\n", err)
			return "", exitParseError
		}
		return strings.TrimRight(string(b), "\r\n"), exitOK
	}
	if flag != "" {
		return flag, exitOK
	}
	fmt.Fprintln(stderr, "password required (--password or --password-stdin)")
	return "", exitUsage
}
