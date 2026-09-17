package parse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devthinker-ai/factura/pkg/parse"
)

func TestDetectFormat_XRechnungCII(t *testing.T) {
	data := readTD(t, "xrechnung-302-cii.xml")
	if got := parse.DetectFormat(data); got != parse.FormatXRechnungCII {
		t.Fatalf("DetectFormat = %q, want xrechnung_cii", got)
	}
}

func TestParse_GoldenCII(t *testing.T) {
	data := readTD(t, "xrechnung-302-cii.xml")
	inv, format, err := parse.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if format != parse.FormatXRechnungCII {
		t.Fatalf("format = %q", format)
	}
	if inv.VendorName() != "Provide One GmbH" {
		t.Fatalf("vendor = %q", inv.VendorName())
	}
	if inv.BuyerName() != "Sample Consumer" {
		t.Fatalf("buyer = %q", inv.BuyerName())
	}
	if inv.InvoiceDate() != "2022-02-01" {
		t.Fatalf("date = %q", inv.InvoiceDate())
	}
	if inv.InvoiceNumber() == "" {
		t.Fatal("empty invoice number")
	}
	if inv.TotalCents() <= 0 {
		t.Fatalf("total cents = %d", inv.TotalCents())
	}
}

func TestParse_Error(t *testing.T) {
	data := readTD(t, "parse-error.txt")
	_, format, err := parse.Parse(data)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if format != parse.FormatUnknown {
		t.Fatalf("format = %q", format)
	}
	if _, ok := err.(*parse.Error); !ok {
		t.Fatalf("want *parse.Error, got %T", err)
	}
}

func TestDetectFormat_ZUGFeRDPDF(t *testing.T) {
	data := readTD(t, "zugferd-23.pdf")
	got := parse.DetectFormat(data)
	if got != parse.FormatZUGFeRDPDF && got != parse.FormatFacturXPDF {
		t.Fatalf("DetectFormat = %q", got)
	}
	inv, _, err := parse.Parse(data)
	if err != nil {
		t.Fatalf("Parse PDF: %v", err)
	}
	if inv.VendorName() == "" {
		t.Fatal("empty vendor from PDF")
	}
}

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
	t.Fatalf("cannot find testdata/%s", name)
	return nil
}
