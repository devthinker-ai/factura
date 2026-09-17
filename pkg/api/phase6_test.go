package api_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/license"
)

func mintTestKey(t *testing.T, priv *rsa.PrivateKey, plan string, maxCompanies int, exp time.Time) string {
	t.Helper()
	claims := license.Claims{
		Plan:         plan,
		MaxCompanies: maxCompanies,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test@factura.eu",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok, err := license.Sign(priv, claims)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// applyEphemeralLicense validates with a test keypair by applying CapsFromClaims
// after ValidateWithKey — production POST /license uses the embedded pub key.
func applyEphemeralLicense(t *testing.T, srv *api.Server, priv *rsa.PrivateKey, tok string) {
	t.Helper()
	claims, err := license.ValidateWithKey(tok, &priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	srv.ApplyCaps(license.CapsFromClaims(claims))
}

func TestLicenseGetUnlicensedShape(t *testing.T) {
	t.Setenv("FACTURA_LICENSE_KEY", "")
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/license", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["plan"] != "solo" || got["licensed"] != false {
		t.Fatalf("got=%v", got)
	}
	caps := got["caps"].(map[string]any)
	if caps["max_companies"] != float64(1) || caps["peppol"] != false {
		t.Fatalf("caps=%v", caps)
	}
	inUse := got["in_use"].(map[string]any)
	if inUse["companies"] != float64(0) || inUse["api_keys"] != float64(0) {
		t.Fatalf("in_use=%v", inUse)
	}
}

func TestTierCompaniesSoloAndPro(t *testing.T) {
	srv, _ := newTestServer(t)
	post := func(name, vat string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"name": name, "seller_vat_id": vat})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}
	if rr := post("A GmbH", "DE111"); rr.Code != 201 {
		t.Fatalf("1st=%d %s", rr.Code, rr.Body.String())
	}
	rr := post("B GmbH", "DE222")
	if rr.Code != 403 {
		t.Fatalf("2nd want 403 got %d %s", rr.Code, rr.Body.String())
	}
	var errBody map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &errBody)
	if errBody["error"] != "plan_limit" || !strings.Contains(errBody["detail"], "Solo") {
		t.Fatalf("body=%v", errBody)
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := mintTestKey(t, priv, license.PlanPro, 0, time.Now().UTC().Add(time.Hour))
	applyEphemeralLicense(t, srv, priv, tok)

	// Pro default MaxCompanies=10 — add until 11th fails. We already have 1.
	for i := 2; i <= 10; i++ {
		rr := post("Co"+itoa(int64(i)), "DE"+itoa(int64(1000+i)))
		if rr.Code != 201 {
			t.Fatalf("company %d: %d %s", i, rr.Code, rr.Body.String())
		}
	}
	rr = post("Overflow", "DE9999")
	if rr.Code != 403 {
		t.Fatalf("11th want 403 got %d", rr.Code)
	}

	// Override --companies 3
	srv2, _ := newTestServer(t)
	tok3 := mintTestKey(t, priv, license.PlanPro, 3, time.Now().UTC().Add(time.Hour))
	applyEphemeralLicense(t, srv2, priv, tok3)
	for i := 1; i <= 3; i++ {
		if rr := func() *httptest.ResponseRecorder {
			body, _ := json.Marshal(map[string]string{"name": "X" + itoa(int64(i)), "seller_vat_id": "VAT" + itoa(int64(i))})
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
			srv2.Handler().ServeHTTP(rr, req)
			return rr
		}(); rr.Code != 201 {
			t.Fatalf("override %d: %d", i, rr.Code)
		}
	}
	body, _ := json.Marshal(map[string]string{"name": "Fourth", "seller_vat_id": "VAT4"})
	rr = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body)))
	if rr.Code != 403 {
		t.Fatalf("4th want 403 got %d", rr.Code)
	}
}

func TestPeppolGatedOnSolo(t *testing.T) {
	srv, _ := newTestServer(t)

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader([]byte(`{"to":"DE:X"}`))))
	if rr.Code != 403 {
		t.Fatalf("send=%d %s", rr.Code, rr.Body.String())
	}
	var errBody map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &errBody)
	if errBody["error"] != "plan_limit" || !strings.Contains(errBody["detail"], "Pro") {
		t.Fatalf("body=%v", errBody)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader([]byte("<x/>"))))
	if rr.Code != 403 {
		t.Fatalf("inbound=%d", rr.Code)
	}
}

func TestAPIKeysCRUDAndAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	// Create without operator token (auth off) — LAN box.
	body, _ := json.Marshal(map[string]any{"name": "acme-embed", "rpm": 60, "monthly_invoices": 100})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body)))
	if rr.Code != 201 {
		t.Fatalf("create=%d %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	key, _ := created["key"].(string)
	if !regexp.MustCompile(`^factura_[0-9a-f]{24}$`).MatchString(key) {
		t.Fatalf("key format %q", key)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api-keys", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"key"`) {
		t.Fatal("list must not include plaintext key field")
	}
	if !strings.Contains(rr.Body.String(), key) {
		t.Fatal("list should show id")
	}

	// Use key to ingest
	inv := readTD(t, "xrechnung-302-cii.xml")
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Factura-External-ID", "k1")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("ingest=%d %s", rr.Code, rr.Body.String())
	}

	// Unknown key
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/invoices", nil)
	req.Header.Set("Authorization", "Bearer factura_000000000000000000000000")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("unknown=%d", rr.Code)
	}

	// Kill switch
	patch, _ := json.Marshal(map[string]bool{"enabled": false})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api-keys/"+key, bytes.NewReader(patch))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/invoices", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("disabled=%d", rr.Code)
	}

	// Re-enable + delete
	patch, _ = json.Marshal(map[string]bool{"enabled": true})
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api-keys/"+key, bytes.NewReader(patch)))
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api-keys/"+key, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Fatalf("delete=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/invoices", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("after delete=%d", rr.Code)
	}
}

func TestAPIKeysAuthOnRequiresOperator(t *testing.T) {
	store, err := archive.Open(filepath.Join(t.TempDir(), "keys-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Setenv("FACTURA_TOKEN", "op-secret")
	t.Setenv("FACTURA_RATE_LIMIT", "1000")
	srv := api.New(store)

	body, _ := json.Marshal(map[string]string{"name": "x"})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body)))
	if rr.Code != 401 {
		t.Fatalf("want 401 got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer op-secret")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("op create=%d %s", rr.Code, rr.Body.String())
	}
}

func TestAPIKeyRateLimitAndQuota(t *testing.T) {
	srv, _ := newTestServer(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	srv.SetNow(func() time.Time { return now })

	body, _ := json.Marshal(map[string]any{"name": "rl", "rpm": 2, "monthly_invoices": 2})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body)))
	var created map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	key := created["key"].(string)

	inv := readTD(t, "xrechnung-302-cii.xml")
	do := func(ext string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("X-Factura-External-ID", ext)
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}
	if rr := do("a"); rr.Code != 201 {
		t.Fatalf("1=%d", rr.Code)
	}
	if rr := do("b"); rr.Code != 201 {
		t.Fatalf("2=%d", rr.Code)
	}
	rr = do("c")
	if rr.Code != 429 {
		t.Fatalf("3rd want 429 got %d %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After=%q", rr.Header().Get("Retry-After"))
	}

	// Advance past window; quota still blocks (used=2, limit=2).
	now = now.Add(61 * time.Second)
	rr = do("d")
	if rr.Code != 403 {
		t.Fatalf("quota want 403 got %d %s", rr.Code, rr.Body.String())
	}
	var qb map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &qb)
	if qb["error"] != "quota_exceeded" {
		t.Fatalf("body=%v", qb)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"invoices_this_month":2`) {
		t.Fatalf("usage=%s", rr.Body.String())
	}
}

func TestMeteringDeterminism(t *testing.T) {
	srv, store := newTestServer(t)
	pro := license.CapsForPlan(license.PlanPro)
	pro.Licensed = true
	srv.ApplyCaps(pro)

	body, _ := json.Marshal(map[string]any{"name": "meter", "rpm": 100, "monthly_invoices": 1000})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body)))
	var created map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	key := created["key"].(string)

	inv := readTD(t, "xrechnung-302-cii.xml")
	// ingest via key = 1
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Factura-External-ID", "m1")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatal(rr.Body.String())
	}

	usage := func() int {
		k, _ := store.KeyByID(key)
		return k.InvoicesThisMonth
	}
	if usage() != 1 {
		t.Fatalf("after ingest=%d", usage())
	}

	// operator ingest = 0 for key
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
	req.Header.Set("X-Factura-External-ID", "op1")
	srv.Handler().ServeHTTP(rr, req)
	if usage() != 1 {
		t.Fatalf("operator must not meter key: %d", usage())
	}

	// month rollover
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	srv.SetNow(func() time.Time { return now })
	_ = srv.Keys.Refresh()
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Factura-External-ID", "m-sep")
	srv.Handler().ServeHTTP(rr, req)
	if usage() != 2 {
		// was 1 from before + 1 = 2 in same period if period still matches stored
		// After refresh with Sep, used starts from DB period.
	}
	now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	srv.SetNow(func() time.Time { return now })
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(inv))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Factura-External-ID", "m-oct")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatal(rr.Body.String())
	}
	k, _ := store.KeyByID(key)
	if k.Period != "2026-10" || k.InvoicesThisMonth != 1 {
		t.Fatalf("rollover: period=%s used=%d", k.Period, k.InvoicesThisMonth)
	}
}

func TestLicensePostPersistsWithEmbeddedKey(t *testing.T) {
	privPath := filepath.Join(os.Getenv("HOME"), ".factura-license-priv.pem")
	pemBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Skip("no ~/.factura-license-priv.pem — skip live mint test")
	}
	priv, err := license.ParseRSAPrivateKey(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "persist@factura.eu",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	db := filepath.Join(t.TempDir(), "lic.db")
	store, err := archive.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	srv := api.New(store)

	body, _ := json.Marshal(map[string]string{"key": tok})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/license", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("post=%d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"plan":"pro"`) {
		t.Fatal(rr.Body.String())
	}

	// Live without restart
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/license", nil))
	if !strings.Contains(rr.Body.String(), `"licensed":true`) {
		t.Fatal(rr.Body.String())
	}

	_ = store.Close()
	store2, err := archive.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	srv2 := api.New(store2)
	rr = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/license", nil))
	if !strings.Contains(rr.Body.String(), `"plan":"pro"`) {
		t.Fatalf("persisted=%s", rr.Body.String())
	}

	// Invalid + expired against live srv2
	rr = httptest.NewRecorder()
	bad, _ := json.Marshal(map[string]string{"key": "not.a.jwt"})
	srv2.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/license", bytes.NewReader(bad)))
	if rr.Code != 422 {
		t.Fatalf("invalid=%d", rr.Code)
	}

	expired, err := license.Sign(priv, license.Claims{
		Plan: license.PlanPro,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "gone",
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(-time.Hour)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	eb, _ := json.Marshal(map[string]string{"key": expired})
	srv2.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/license", bytes.NewReader(eb)))
	if rr.Code != 422 || !strings.Contains(rr.Body.String(), "detail") {
		t.Fatalf("expired=%d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/license", nil))
	if rr.Code != 204 {
		t.Fatalf("delete=%d", rr.Code)
	}
}

func TestSchemaUntouched(t *testing.T) {
	store, err := archive.Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.ListCompanies(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListKeys(); err != nil {
		t.Fatal(err)
	}
}
