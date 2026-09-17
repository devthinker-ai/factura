package generate

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 168, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPh11_BrandedRoundTrip(t *testing.T) {
	inv, err := model.FromJSON(consoleShapeJSON("standard", "R-BRAND-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := Template{
		Logo:       testJPEG(t),
		HasLogo:    true,
		Accent:     [3]uint8{0x1E, 0x5A, 0xA8},
		HasAccent:  true,
		HeaderText: "Provide One GmbH — Branding",
		FooterText: "Impressum line one\nHRB 12345 Mannheim",
	}
	dir := t.TempDir()
	xr, zf, err := GenerateWith(*inv, dir, &tmpl)
	if err != nil {
		t.Fatalf("GenerateWith: %v", err)
	}
	if _, err := os.Stat(xr); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(zf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("DCTDecode")) {
		t.Fatal("expected DCTDecode in hybrid PDF")
	}
	foundAccent := false
	for _, tok := range []string{"0.118", "0.353", "0.659"} {
		if bytes.Contains(pdf, []byte(tok)) {
			foundAccent = true
			break
		}
	}
	if !foundAccent {
		t.Fatal("expected accent RGB floats in PDF content")
	}
	if !bytes.Contains(pdf, []byte("Impressum line one")) {
		t.Fatal("expected footer text in PDF")
	}
	if !bytes.Contains(pdf, []byte("Payable:")) {
		t.Fatal("expected Payable: label")
	}
	if rep := validate.ValidateBytes(pdf); !rep.Valid {
		t.Fatalf("self-check hybrid: %s %+v", rep.Summary, rep.Violations)
	}
}

func TestPh11_UnbrandedNoDCT(t *testing.T) {
	inv, err := model.FromJSON(consoleShapeJSON("standard", "R-PLAIN-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_, zf, err := GenerateWith(*inv, dir, nil)
	if err != nil {
		t.Fatalf("GenerateWith: %v", err)
	}
	pdf, err := os.ReadFile(zf)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte("DCTDecode")) {
		t.Fatal("unbranded PDF must not embed DCTDecode logo")
	}
	if !bytes.Contains(pdf, []byte("Bezeichnung / Item")) {
		t.Fatal("expected table header text")
	}
	if !bytes.Contains(pdf, []byte("Payable:")) {
		t.Fatal("expected Payable:")
	}
	if rep := validate.ValidateBytes(pdf); !rep.Valid {
		t.Fatalf("validate: %s", rep.Summary)
	}
	_, fmtName, err := parse.Parse(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if fmtName != parse.FormatZUGFeRDPDF && fmtName != parse.FormatFacturXPDF {
		t.Fatalf("format=%s", fmtName)
	}
}

func TestPh11_MultiPage40Lines(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(consoleShapeJSON("standard", "R-40", nil), &doc); err != nil {
		t.Fatal(err)
	}
	lines := make([]map[string]any, 40)
	for i := 0; i < 40; i++ {
		lines[i] = map[string]any{
			"quantity": "1",
			"item":     map[string]string{"name": "Item", "price": "10.00"},
			"taxes":    []map[string]string{{"cat": "VAT", "rate": "general"}},
		}
	}
	doc["lines"] = lines
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := model.FromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_, zf, err := GenerateWith(*inv, dir, nil)
	if err != nil {
		t.Fatalf("GenerateWith: %v", err)
	}
	pdf, err := os.ReadFile(zf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Count 2")) {
		t.Fatalf("expected /Count 2, snippet=%s", ph11Snippet(pdf, "/Count"))
	}
	if c := bytes.Count(pdf, []byte("Bezeichnung / Item")); c < 2 {
		t.Fatalf("expected repeated table header, count=%d", c)
	}
	if rep := validate.ValidateBytes(pdf); !rep.Valid {
		t.Fatalf("self-check: %s %+v", rep.Summary, rep.Violations)
	}
}

func ph11Snippet(b []byte, needle string) string {
	i := bytes.Index(b, []byte(needle))
	if i < 0 {
		return "(missing)"
	}
	end := i + 40
	if end > len(b) {
		end = len(b)
	}
	return string(b[i:end])
}
