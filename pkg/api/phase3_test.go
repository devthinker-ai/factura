package api_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/web"
)

func TestHealth_Version(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" {
		t.Fatalf("status=%q", got["status"])
	}
	if got["version"] != "dev" {
		t.Fatalf("version=%q want dev", got["version"])
	}
	if got["plan"] != "solo" {
		t.Fatalf("plan=%q want solo (unlicensed)", got["plan"])
	}
}

func TestRoot_Redirect(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetSPA(web.HandlerFromFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><div id="root"></div><script src="/app/assets/x.js"></script>`)},
		"assets/x.js": &fstest.MapFile{Data: []byte(`console.log(1)`)},
	}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/app/" {
		t.Fatalf("Location=%q", loc)
	}
	if !strings.Contains(rr.Body.String(), "/app/") {
		t.Fatalf("body missing meta-refresh: %s", rr.Body.String())
	}
}

func TestSPA_Serving(t *testing.T) {
	index := []byte(`<!DOCTYPE html><html><body><div id="root"></div><script type="module" src="/app/assets/index-abc123.js"></script></body></html>`)
	asset := []byte(`export const app = 1;`)
	srv, _ := newTestServer(t)
	srv.SetSPA(web.HandlerFromFS(fstest.MapFS{
		"index.html":            &fstest.MapFile{Data: index},
		"assets/index-abc123.js": &fstest.MapFile{Data: asset},
	}))
	h := srv.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app/", nil))
	if rr.Code != 200 {
		t.Fatalf("/app/ status=%d body=%s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type=%q", ct)
	}
	if !strings.Contains(rr.Body.String(), `id="root"`) {
		t.Fatal("missing root div")
	}
	if !strings.Contains(rr.Body.String(), "assets/index-abc123.js") {
		t.Fatal("missing script tag")
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("index Cache-Control=%q", cc)
	}

	// Client route → same index.html
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app/invoices/42", nil))
	if rr.Code != 200 {
		t.Fatalf("client route status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `id="root"`) {
		t.Fatal("SPA fallback missing root")
	}

	// Hashed asset
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app/assets/index-abc123.js", nil))
	if rr.Code != 200 {
		t.Fatalf("asset status=%d", rr.Code)
	}
	if rr.Body.String() != string(asset) {
		t.Fatalf("asset bytes mismatch: %q", rr.Body.String())
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset Cache-Control=%q", cc)
	}
}

func TestSPA_PlaceholderGuard(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetSPA(web.HandlerFromFS(fstest.MapFS{
		"placeholder.txt": &fstest.MapFile{Data: []byte("run make frontend")},
	}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "make frontend") {
		t.Fatalf("hint missing: %s", rr.Body.String())
	}
}

func TestVendors(t *testing.T) {
	srv, store := newTestServer(t)
	body := readTD(t, "xrechnung-302-cii.xml")
	if _, err := store.IngestWithSource(body, "vendor-a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestWithSource(body, "vendor-b", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestWithSource([]byte("nope"), "blank-vendor", ""); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/vendors", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Vendors []string `json:"vendors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Vendors) != 1 {
		t.Fatalf("want 1 vendor (blank excluded), got %v", got.Vendors)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/vendors?limit=1", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got.Vendors) != 1 {
		t.Fatalf("limit: %v", got.Vendors)
	}

	// Auth required when token set
	t.Setenv("FACTURA_TOKEN", "secret")
	t.Setenv("FACTURA_RATE_LIMIT", "1000")
	authed := api.New(store)
	rr = httptest.NewRecorder()
	authed.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/vendors", nil))
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/invoices/vendors", nil)
	req.Header.Set("Authorization", "Bearer secret")
	authed.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("authed=%d %s", rr.Code, rr.Body.String())
	}
}

func TestSPA_EmbedDistExists(t *testing.T) {
	// Production embed must at least have the placeholder or a real index.
	_, err := fs.Stat(web.DistFS(), "placeholder.txt")
	indexErr := func() error {
		_, e := fs.Stat(web.DistFS(), "index.html")
		return e
	}()
	if err != nil && indexErr != nil {
		t.Fatal("embed has neither placeholder.txt nor index.html")
	}
}
