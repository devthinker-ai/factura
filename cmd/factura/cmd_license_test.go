package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/license"
)

func TestLicenseCheckExitCodes(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := cmdLicenseCheck(nil, &out, &errBuf)
	if code != exitUnlicensed {
		t.Fatalf("unlicensed exit=%d want %d body=%s", code, exitUnlicensed, out.String())
	}
	if !strings.Contains(out.String(), `"licensed":false`) {
		t.Fatal(out.String())
	}

	privPath := filepath.Join(os.Getenv("HOME"), ".factura-license-priv.pem")
	pemBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Skip("no private key")
	}
	priv, err := license.ParseRSAPrivateKey(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "cli@test",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = cmdLicenseCheck([]string{"--license", tok}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("licensed exit=%d body=%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"plan":"pro"`) {
		t.Fatal(out.String())
	}
}

func TestAPIKeysCLIRoundTrip(t *testing.T) {
	db := filepath.Join(t.TempDir(), "keys.db")
	var out, errBuf bytes.Buffer
	code := cmdAPIKeysCreate([]string{"--name", "cli", "--db", db}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("create=%d %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "factura_") {
		t.Fatal(out.String())
	}
	store, err := archive.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := store.ListKeys()
	_ = store.Close()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	out.Reset()
	code = cmdAPIKeysList([]string{"--db", db}, &out, &errBuf)
	if code != exitOK || !strings.Contains(out.String(), keys[0].ID) {
		t.Fatalf("list=%d %s", code, out.String())
	}
	code = cmdAPIKeysDelete([]string{"--id", keys[0].ID, "--db", db}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("delete=%d", code)
	}
}
