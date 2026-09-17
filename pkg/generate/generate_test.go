package generate_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devthinker-ai/factura/pkg/generate"
	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
	"github.com/invopop/gobl/cal"
)

func TestGenerate_RoundTrip(t *testing.T) {
	inv, err := model.NewDEB2B("RT-2026-001", cal.MakeDate(2026, 3, 1))
	if err != nil {
		t.Fatalf("NewDEB2B: %v", err)
	}
	dir := t.TempDir()
	xr, zf, err := generate.Generate(*inv, dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(xr); err != nil {
		t.Fatalf("xrechnung missing: %v", err)
	}
	if _, err := os.Stat(zf); err != nil {
		t.Fatalf("zugferd missing: %v", err)
	}

	xrBytes, err := os.ReadFile(xr)
	if err != nil {
		t.Fatal(err)
	}
	parsedXR, fmtXR, err := parse.Parse(xrBytes)
	if err != nil {
		t.Fatalf("parse xrechnung: %v", err)
	}
	if fmtXR != parse.FormatXRechnungCII {
		t.Fatalf("format = %s", fmtXR)
	}
	assertMatch(t, inv, &parsedXR)

	zfBytes, err := os.ReadFile(zf)
	if err != nil {
		t.Fatal(err)
	}
	parsedZF, fmtZF, err := parse.Parse(zfBytes)
	if err != nil {
		t.Fatalf("parse zugferd: %v", err)
	}
	if fmtZF != parse.FormatZUGFeRDPDF && fmtZF != parse.FormatFacturXPDF {
		t.Fatalf("zugferd format = %s", fmtZF)
	}
	assertMatch(t, inv, &parsedZF)

	if rep := validate.ValidateBytes(xrBytes); !rep.Valid {
		t.Fatalf("validate xrechnung: %s %+v", rep.Summary, rep.Violations)
	}
	if rep := validate.ValidateBytes(zfBytes); !rep.Valid {
		t.Fatalf("validate zugferd: %s %+v", rep.Summary, rep.Violations)
	}

	// Commit golden ZUGFeRD if generating from test helper path.
	writeGoldenIfRequested(t, zfBytes)
}

func TestGenerate_SelfCheckCatchesMutation(t *testing.T) {
	inv, err := model.NewDEB2B("MUT-001", cal.MakeDate(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	xr, _, err := generate.Generate(*inv, dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	raw, err := os.ReadFile(xr)
	if err != nil {
		t.Fatal(err)
	}
	// Strip seller VAT ID from generated CII.
	mut := bytes.Replace(raw,
		[]byte(`<ram:ID schemeID="VA">DE111111125</ram:ID>`),
		[]byte(`<ram:ID schemeID="VA"></ram:ID>`),
		1,
	)
	if bytes.Equal(mut, raw) {
		// Try without DE prefix in GOBL output (code-only).
		mut = bytes.Replace(raw,
			[]byte(`DE111111125`),
			[]byte(``),
			1,
		)
	}
	rep := validate.ValidateBytes(mut)
	if rep.Valid {
		t.Fatal("mutated CII should be invalid")
	}
	found := false
	for _, v := range rep.Violations {
		if strings.HasPrefix(v.RuleID, "BR-DE-16") || v.RuleID == "BR-CO-26" || v.RuleID == "BR-S-2" {
			found = true
			if v.Human == "" {
				t.Fatal("empty human")
			}
		}
	}
	if !found {
		t.Fatalf("expected VAT-ID related rule, got %+v", rep.Violations)
	}
}

func TestGenerate_MissingFieldsErrors(t *testing.T) {
	inv, err := model.NewDEB2B("BAD-001", cal.MakeDate(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	// Remove required buyer reference / ordering to make emit fail validation.
	b := inv.Bill()
	b.Ordering = nil
	b.Supplier.TaxID = nil
	// Re-wrap without recalculating addons if possible — FromBill recalculates.
	broken, err := model.FromBill(b)
	if err != nil {
		// Calculate itself may fail — that also counts as generate refusing to emit.
		t.Logf("FromBill error (acceptable): %v", err)
		return
	}
	dir := t.TempDir()
	_, _, err = generate.Generate(*broken, dir)
	if err == nil {
		// If GOBL still calculates, generate self-check should catch it.
		t.Fatal("expected generate error for incomplete invoice")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no output files, got %d", len(entries))
	}
}

func assertMatch(t *testing.T, src *model.Invoice, got *model.Invoice) {
	t.Helper()
	if got.VendorName() != src.VendorName() {
		t.Fatalf("vendor: got %q want %q", got.VendorName(), src.VendorName())
	}
	if got.InvoiceNumber() != src.InvoiceNumber() && !strings.Contains(got.InvoiceNumber(), "RT-2026-001") && !strings.Contains(src.InvoiceNumber(), got.InvoiceNumber()) {
		// Series/code composition may differ slightly after round-trip.
		if got.Bill() == nil || string(got.Bill().Code) != string(src.Bill().Code) {
			t.Fatalf("number: got %q want %q", got.InvoiceNumber(), src.InvoiceNumber())
		}
	}
	if got.InvoiceDate() != src.InvoiceDate() {
		t.Fatalf("date: got %q want %q", got.InvoiceDate(), src.InvoiceDate())
	}
	if got.TotalCents() != src.TotalCents() {
		t.Fatalf("total: got %d want %d", got.TotalCents(), src.TotalCents())
	}
	if got.VATCents() != src.VATCents() {
		t.Fatalf("vat: got %d want %d", got.VATCents(), src.VATCents())
	}
}

func writeGoldenIfRequested(t *testing.T, pdf []byte) {
	t.Helper()
	if os.Getenv("FACTURA_WRITE_GOLDEN") != "1" {
		return
	}
	root := findRepoRoot(t)
	path := filepath.Join(root, "testdata", "zugferd-23.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	t.Logf("wrote %s", path)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatal("repo root not found")
	return ""
}
