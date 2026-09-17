package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/devthinker-ai/factura/pkg/license"
	"github.com/devthinker-ai/factura/pkg/session"
)

func jpegB64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 0x1E, G: 0x5A, B: 0xA8, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func pngB64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestPh11_BrandingCRUD(t *testing.T) {
	srv, store := newTestServer(t)
	srv.ApplyCaps(license.CapsForPlan(license.PlanPro))
	c, err := store.AddCompany("Brand GmbH", "DE111", "")
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{
		"branding": map[string]any{
			"logo_b64":    jpegB64(t),
			"logo_mime":   "image/jpeg",
			"accent":      "#1E5AA8",
			"header_text": "Header",
			"footer_text": "Footer line",
		},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/companies/"+strconv.FormatInt(c.ID, 10), bytes.NewReader(body))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("patch branding: %d %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	br := got["branding"].(map[string]any)
	if br["logo_mime"] != "image/jpeg" {
		t.Fatalf("mime=%v", br["logo_mime"])
	}
	if br["logo_b64"] == "" {
		t.Fatal("expected logo_b64 on GET-shaped patch response")
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/companies/"+strconv.FormatInt(c.ID, 10), nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	br = got["branding"].(map[string]any)
	if br["logo_mime"] != "image/jpeg" || br["accent"] != "#1E5AA8" {
		t.Fatalf("get=%v", br)
	}

	body, _ = json.Marshal(map[string]any{
		"branding": map[string]any{
			"logo_b64":  pngB64(t),
			"logo_mime": "image/png",
			"accent":    "#00AA00",
		},
	})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/companies/"+strconv.FormatInt(c.ID, 10), bytes.NewReader(body))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	br = got["branding"].(map[string]any)
	if br["logo_mime"] != "image/jpeg" {
		t.Fatalf("png normalize mime=%v", br["logo_mime"])
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/companies", nil))
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	list := got["companies"].([]any)
	row := list[0].(map[string]any)
	if row["has_logo"] != true {
		t.Fatalf("has_logo=%v", row["has_logo"])
	}
	lbr := row["branding"].(map[string]any)
	if _, ok := lbr["logo_b64"]; ok {
		t.Fatal("list must omit logo_b64")
	}

	if _, err := store.AddCompany("Other", "DE222", ""); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/companies/"+strconv.FormatInt(c.ID, 10), bytes.NewReader([]byte("{}")))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["is_default"] != true {
		t.Fatalf("is_default=%v", got["is_default"])
	}
	br = got["branding"].(map[string]any)
	if br["logo_b64"] == "" {
		t.Fatal("set-default must leave branding intact")
	}
}

func TestPh11_BrandingValidation(t *testing.T) {
	srv, store := newTestServer(t)
	c, err := store.AddCompany("V GmbH", "", "")
	if err != nil {
		t.Fatal(err)
	}
	patch := func(branding map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"branding": branding})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/companies/"+strconv.FormatInt(c.ID, 10), bytes.NewReader(body))
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}
	big := make([]byte, 520_000)
	for i := range big {
		big[i] = 'A'
	}
	rr := patch(map[string]any{
		"logo_b64":  base64.StdEncoding.EncodeToString(big),
		"logo_mime": "image/jpeg",
	})
	if rr.Code != 422 {
		t.Fatalf("oversize want 422 got %d %s", rr.Code, rr.Body.String())
	}
	rr = patch(map[string]any{"accent": "red"})
	if rr.Code != 422 {
		t.Fatalf("bad accent: %d", rr.Code)
	}
	rr = patch(map[string]any{"footer_text": strings.Repeat("x", 501)})
	if rr.Code != 422 {
		t.Fatalf("long footer: %d", rr.Code)
	}
}

func TestPh11_BrandingRoles(t *testing.T) {
	srv, store := newTestServer(t)
	c, err := store.AddCompany("Role Co", "", "")
	if err != nil {
		t.Fatal(err)
	}
	viewer := addUser(t, store, "v@t.de", "viewer", "password12")
	editor := addUser(t, store, "e@t.de", "editor", "password12")
	srv.BumpUsersCache(true)
	vTok, err := srv.Sessions.Issue(session.User{ID: viewer.ID, Email: viewer.Email, Role: viewer.Role})
	if err != nil {
		t.Fatal(err)
	}
	eTok, err := srv.Sessions.Issue(session.User{ID: editor.ID, Email: editor.Email, Role: editor.Role})
	if err != nil {
		t.Fatal(err)
	}

	path := "/companies/" + strconv.FormatInt(c.ID, 10)
	rr := doJSON(t, srv, http.MethodGet, path, nil, vTok)
	if rr.Code != 200 {
		t.Fatalf("viewer GET: %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, srv, http.MethodPatch, path, map[string]any{
		"branding": map[string]string{"accent": "#112233"},
	}, vTok)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("viewer PATCH want 401 forbidden got %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, srv, http.MethodPatch, path, map[string]any{
		"branding": map[string]string{"accent": "#112233"},
	}, eTok)
	if rr.Code != 200 {
		t.Fatalf("editor PATCH: %d %s", rr.Code, rr.Body.String())
	}
}

func TestPh11_GenerateXMLInvariant(t *testing.T) {
	srv, store := newTestServer(t)
	srv.ApplyCaps(license.CapsForPlan(license.PlanPro))
	plainCo, err := store.AddCompany("Plain Co", "DE111", "")
	if err != nil {
		t.Fatal(err)
	}
	brandCo, err := store.AddCompany("Brand Co", "DE222", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"branding": map[string]any{
			"logo_b64":    jpegB64(t),
			"logo_mime":   "image/jpeg",
			"accent":      "#1E5AA8",
			"footer_text": "Foot",
		},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/companies/"+strconv.FormatInt(brandCo.ID, 10), bytes.NewReader(body))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}

	invBody := consoleInvoiceJSON(t)
	gen := func(qs string) map[string]any {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/invoices/generate?"+qs, bytes.NewReader(invBody))
		req.Header.Set("Content-Type", "application/json")
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != 201 {
			t.Fatalf("generate %s: %d %s", qs, rr.Code, rr.Body.String())
		}
		var got map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &got)
		return got
	}
	// Explicit company_id: unbranded vs branded (default would also brand once saved).
	plain := gen("format=both&company_id=" + strconv.FormatInt(plainCo.ID, 10))
	branded := gen("format=both&company_id=" + strconv.FormatInt(brandCo.ID, 10))
	if plain["self_check"] != true || branded["self_check"] != true {
		t.Fatal("self_check")
	}
	if plain["xrechnung"] != branded["xrechnung"] {
		t.Fatal("XRechnung XML must be byte-identical with/without branding")
	}
	if plain["zugferd_pdf_base64"] == branded["zugferd_pdf_base64"] {
		t.Fatal("branded PDF should differ from unbranded")
	}
	_ = plainCo
}

func consoleInvoiceJSON(t *testing.T) []byte {
	t.Helper()
	inv := map[string]any{
		"$schema":    "https://gobl.org/draft-0/bill/invoice",
		"$regime":    "DE",
		"$addons":    []string{"de-xrechnung-v3", "de-zugferd-v2"},
		"type":       "standard",
		"code":       "R-API-1",
		"issue_date": "2026-09-01",
		"currency":   "EUR",
		"supplier": map[string]any{
			"name": "Provide One GmbH",
			"tax_id": map[string]string{
				"country": "DE", "code": "111111125",
			},
			"addresses": []map[string]string{{
				"street": "Dietmar-Hopp-Allee", "locality": "Walldorf",
				"code": "69190", "country": "DE",
			}},
			"emails":  []map[string]string{{"addr": "billing@example.com"}},
			"inboxes": []map[string]string{{"email": "billing@example.com"}},
			"people": []map[string]any{{
				"name":       map[string]string{"given": "Ada"},
				"telephones": []map[string]string{{"num": "+49100200300"}},
				"emails":     []map[string]string{{"addr": "billing@example.com"}},
			}},
		},
		"customer": map[string]any{
			"name": "Sample Consumer",
			"tax_id": map[string]string{
				"country": "DE", "code": "282741168",
			},
			"addresses": []map[string]string{{
				"street": "Werner-Heisenberg-Allee", "locality": "Munchen",
				"code": "80939", "country": "DE",
			}},
			"emails":  []map[string]string{{"addr": "customer@example.com"}},
			"inboxes": []map[string]string{{"email": "customer@example.com"}},
		},
		"lines": []map[string]any{{
			"quantity": "1",
			"item":     map[string]string{"name": "Consulting", "price": "100.00"},
			"taxes":    []map[string]string{{"cat": "VAT", "rate": "general"}},
		}},
		"ordering": map[string]string{"code": "NA"},
		"payment": map[string]any{
			"instructions": map[string]any{
				"key": "credit-transfer+sepa",
				"credit_transfer": []map[string]string{{
					"iban": "DE89370400440532013000",
				}},
			},
			"terms": map[string]string{"notes": "14 Tage netto"},
		},
	}
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
