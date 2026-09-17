package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/invopop/gobl/cal"
)

func readTD(t *testing.T, name string) []byte {
	t.Helper()
	candidates := []string{
		filepath.Join("testdata", name),
		filepath.Join("..", "..", "testdata", name),
	}
	wd, _ := os.Getwd()
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidates = append(candidates, filepath.Join(dir, "testdata", name))
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err == nil {
			return b
		}
	}
	t.Fatalf("testdata/%s not found", name)
	return nil
}

func newTestServer(t *testing.T) (*api.Server, *archive.Store) {
	t.Helper()
	t.Setenv("FACTURA_TOKEN", "")
	t.Setenv("FACTURA_RATE_LIMIT", "")
	store, err := archive.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return api.New(store), store
}

func TestIngest_HappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	body := readTD(t, "xrechnung-302-cii.xml")
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("X-Factura-Source", "test")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "valid" {
		t.Fatalf("status=%v", got["status"])
	}
	if id, _ := got["id"].(float64); id < 1 {
		t.Fatalf("id=%v", got["id"])
	}
	if got["vendor_name"] == "" || got["invoice_number"] == "" {
		t.Fatalf("missing vendor/number: %+v", got)
	}
	if got["report"] == nil {
		t.Fatal("missing report")
	}
}

func TestIngest_Invalid(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(readTD(t, "broken-amounts.xml")))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["status"] != "invalid" {
		t.Fatalf("status=%v", got["status"])
	}
	rep, _ := got["report"].(map[string]any)
	violations, _ := rep["violations"].([]any)
	found := false
	for _, v := range violations {
		m, _ := v.(map[string]any)
		if m["rule_id"] == "BR-CO-15" || m["rule_id"] == "BR-CO-16" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected BR-CO-15/16 in %+v", violations)
	}
}

func TestIngest_Unparseable(t *testing.T) {
	srv, store := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(readTD(t, "parse-error.txt")))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415, got %d %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	id, _ := got["id"].(float64)
	if id < 1 {
		t.Fatalf("missing id: %s", rr.Body.String())
	}
	if got["detail"] == nil || got["detail"] == "" {
		t.Fatal("missing detail")
	}
	row, err := store.Get(int64(id))
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != archive.StatusParseError {
		t.Fatalf("status=%s", row.Status)
	}
}

func TestIngest_Idempotency(t *testing.T) {
	srv, _ := newTestServer(t)
	body := readTD(t, "xrechnung-302-cii.xml")
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
		req.Header.Set("X-Factura-External-ID", "acme-INV-1")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}
	rr1 := post()
	if rr1.Code != 201 {
		t.Fatalf("first=%d %s", rr1.Code, rr1.Body.String())
	}
	var first map[string]any
	_ = json.Unmarshal(rr1.Body.Bytes(), &first)
	rr2 := post()
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second=%d %s", rr2.Code, rr2.Body.String())
	}
	var second map[string]any
	_ = json.Unmarshal(rr2.Body.Bytes(), &second)
	if second["id"] != first["id"] {
		t.Fatalf("ids differ: %v vs %v", first["id"], second["id"])
	}
}

func TestAuth(t *testing.T) {
	store, err := archive.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Setenv("FACTURA_TOKEN", "secret")
	t.Setenv("FACTURA_RATE_LIMIT", "1000")
	srv := api.New(store)

	// health open
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != 200 {
		t.Fatalf("health=%d", rr.Code)
	}

	body := readTD(t, "xrechnung-302-cii.xml")
	// no header
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("no auth=%d", rr.Code)
	}
	// wrong
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("wrong=%d", rr.Code)
	}
	// right
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("ok=%d %s", rr.Code, rr.Body.String())
	}

	// without token env — open
	t.Setenv("FACTURA_TOKEN", "")
	open := api.New(store)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	req.Header.Set("X-Factura-External-ID", "open-1")
	open.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("open=%d", rr.Code)
	}
}

func TestRateLimit(t *testing.T) {
	store, err := archive.Open(filepath.Join(t.TempDir(), "rl.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Setenv("FACTURA_TOKEN", "tok")
	t.Setenv("FACTURA_RATE_LIMIT", "5")
	srv := api.New(store)
	body := readTD(t, "xrechnung-302-cii.xml")
	saw429 := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("X-Factura-External-ID", "rl-"+strconv.Itoa(i))
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			saw429 = true
			if rr.Header().Get("Retry-After") == "" {
				t.Fatal("missing Retry-After")
			}
			break
		}
	}
	if !saw429 {
		t.Fatal("expected at least one 429")
	}
}

func TestReadEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()
	post := func(name, id string) {
		req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(readTD(t, name)))
		req.Header.Set("X-Factura-External-ID", id)
		req.Header.Set("X-Factura-Source", "api-test")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 201 && rr.Code != 415 {
			t.Fatalf("post %s: %d %s", name, rr.Code, rr.Body.String())
		}
	}
	post("xrechnung-302-cii.xml", "r-valid")
	post("broken-amounts.xml", "r-invalid")
	post("parse-error.txt", "r-parse")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var list struct {
		Rows  []archive.Row `json:"rows"`
		Total int           `json:"total"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if list.Total != 3 || len(list.Rows) != 3 {
		t.Fatalf("total=%d len=%d", list.Total, len(list.Rows))
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices?status=invalid", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if list.Total != 1 {
		t.Fatalf("invalid total=%d", list.Total)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices?q=SAMPLE", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if list.Total < 1 {
		t.Fatal("q=SAMPLE empty")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices?since=2022-02-01&until=2022-02-02", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if list.Total < 1 {
		t.Fatal("date filter empty")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices?limit=2", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if list.Total != 3 || len(list.Rows) != 2 {
		t.Fatalf("limit: total=%d len=%d", list.Total, len(list.Rows))
	}

	// find valid id
	var validID int64
	for _, r := range list.Rows {
		if r.Status == "valid" || r.Status == "invalid" {
			validID = r.ID
			break
		}
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices?status=valid", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	validID = list.Rows[0].ID

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/"+itoa(validID), nil))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var row archive.Row
	_ = json.Unmarshal(rr.Body.Bytes(), &row)
	if len(row.DocGOBL) < 2 {
		t.Fatal("missing doc_gobl_json")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/99999", nil))
	if rr.Code != 404 {
		t.Fatalf("404 got %d", rr.Code)
	}

	orig := readTD(t, "xrechnung-302-cii.xml")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/"+itoa(validID)+"/original", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), orig) {
		t.Fatal("original mismatch")
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "xml") {
		t.Fatalf("ct=%s", rr.Header().Get("Content-Type"))
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/invoices/export", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	recs, err := csv.NewReader(rr.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) < 2 {
		t.Fatal("csv empty")
	}
	if recs[0][len(recs[0])-1] != "invoice_type" {
		t.Fatalf("header=%v", recs[0])
	}
	if recs[0][len(recs[0])-2] != "source" {
		t.Fatalf("header missing source before invoice_type: %v", recs[0])
	}
}

func TestGenerate(t *testing.T) {
	srv, _ := newTestServer(t)
	inv, err := model.NewDEB2B("API-GEN-1", cal.MakeDate(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	gobl, err := inv.JSON()
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/invoices/generate", bytes.NewReader(gobl))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	xr, _ := got["xrechnung"].(string)
	if !strings.Contains(xr, "API-GEN-1") {
		t.Fatal("xrechnung missing number")
	}
	b64, _ := got["zugferd_pdf_base64"].(string)
	pdf, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("pdf: %v prefix=%q", err, pdf[:min(4, len(pdf))])
	}
	if got["self_check"] != true {
		t.Fatal("self_check")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices/generate?format=xrechnung&download=1", bytes.NewReader(gobl))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "API-GEN-1") {
		t.Fatalf("download xr: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices/generate", bytes.NewReader([]byte(`{bad`)))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 422 {
		t.Fatalf("malformed=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/invoices/generate/preview", bytes.NewReader(gobl))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("preview=%d", rr.Code)
	}
}

func TestConcurrencyIdempotent(t *testing.T) {
	srv, store := newTestServer(t)
	body := readTD(t, "xrechnung-302-cii.xml")
	var created, conflicted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
			req.Header.Set("X-Factura-External-ID", "concurrent-1")
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			switch rr.Code {
			case 201:
				created.Add(1)
			case 409:
				conflicted.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || conflicted.Load() != 19 {
		t.Fatalf("created=%d conflicted=%d", created.Load(), conflicted.Load())
	}
	n, err := store.Count(archive.Filter{})
	if err != nil || n != 1 {
		t.Fatalf("rows=%d err=%v", n, err)
	}
}

func TestBodyTooLarge(t *testing.T) {
	srv, store := newTestServer(t)
	big := bytes.Repeat([]byte("x"), (10<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(big))
	req.ContentLength = int64(len(big))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", rr.Code)
	}
	n, _ := store.Count(archive.Filter{})
	if n != 0 {
		t.Fatalf("stored %d rows", n)
	}
}

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"ok"`) {
		t.Fatal(rr.Body.String())
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
