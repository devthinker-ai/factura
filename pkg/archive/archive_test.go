package archive_test

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/devthinker-ai/factura/pkg/archive"
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
	t.Fatalf("cannot find testdata/%s", name)
	return nil
}

func TestArchive_IngestListSearchExport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := archive.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	valid := readTD(t, "xrechnung-302-cii.xml")
	invalid := readTD(t, "broken-amounts.xml")
	bad := readTD(t, "parse-error.txt")

	r1, err := store.Ingest(valid, "ext-valid")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != archive.StatusValid {
		t.Fatalf("status=%s", r1.Status)
	}
	r2, err := store.Ingest(invalid, "ext-invalid")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Status != archive.StatusInvalid {
		t.Fatalf("status=%s", r2.Status)
	}
	r3, err := store.Ingest(bad, "ext-parse")
	if err != nil {
		t.Fatal(err)
	}
	if r3.Status != archive.StatusParseError {
		t.Fatalf("status=%s", r3.Status)
	}

	all, err := store.List(archive.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("list len=%d", len(all))
	}
	validOnly, err := store.List(archive.Filter{Status: archive.StatusValid})
	if err != nil {
		t.Fatal(err)
	}
	if len(validOnly) != 1 {
		t.Fatalf("valid filter=%d", len(validOnly))
	}
	byVendor, err := store.List(archive.Filter{Vendor: "Provide"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byVendor) < 1 {
		t.Fatal("vendor filter empty")
	}
	found, err := store.Search("SAMPLE", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 1 {
		t.Fatal("search by number failed")
	}

	var buf bytes.Buffer
	if err := store.Export(archive.Filter{}, &buf); err != nil {
		t.Fatal(err)
	}
	cr := csv.NewReader(&buf)
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 { // header + 3
		t.Fatalf("csv rows=%d", len(records))
	}
	wantHeader := []string{"id", "external_id", "format", "status", "vendor", "invoice_number", "invoice_date", "total", "vat", "currency", "source", "invoice_type"}
	for i, h := range wantHeader {
		if records[0][i] != h {
			t.Fatalf("header[%d]=%q want %q", i, records[0][i], h)
		}
	}

	orig, err := store.Original(r1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, valid) {
		t.Fatal("original bytes mismatch")
	}
}
