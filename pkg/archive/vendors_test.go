package archive_test

import (
	"path/filepath"
	"testing"

	"github.com/devthinker-ai/factura/pkg/archive"
)

func TestDistinctVendors(t *testing.T) {
	store, err := archive.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	body := readTD(t, "xrechnung-302-cii.xml")
	if _, err := store.IngestWithSource(body, "v-a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestWithSource(body, "v-b", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestWithSource([]byte("not-xml"), "blank", ""); err != nil {
		t.Fatal(err)
	}

	vendors, err := store.DistinctVendors(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(vendors) != 1 {
		t.Fatalf("want 1 vendor, got %v", vendors)
	}
	if vendors[0] == "" {
		t.Fatal("empty vendor")
	}

	limited, err := store.DistinctVendors(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit=1: %v", limited)
	}
}
