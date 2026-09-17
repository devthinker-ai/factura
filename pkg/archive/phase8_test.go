package archive

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestPh8_HashChainIngest(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Three valid-ish ingestions via parse_error garbage still join the chain.
	for i := 0; i < 3; i++ {
		_, err := store.Ingest([]byte("not-an-invoice-"+string(rune('a'+i))), "")
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM chain_links`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("chain_links=%d want 3", n)
	}

	rep, err := store.VerifyChain()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.LinkCount != 3 || rep.Head == "" {
		t.Fatalf("verify: %+v", rep)
	}

	// Genesis on link 1.
	var prev string
	if err := store.db.QueryRow(`SELECT prev_hash FROM chain_links ORDER BY id ASC LIMIT 1`).Scan(&prev); err != nil {
		t.Fatal(err)
	}
	if prev != chainGenesis() {
		t.Fatalf("first prev_hash=%s want genesis", prev)
	}
}

func TestPh8_ParseErrorJoinsChain(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	row, err := store.Ingest([]byte("%%%garbage%%%"), "ext-pe")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusParseError {
		t.Fatalf("status=%s", row.Status)
	}
	ev, err := store.EvidenceFor(row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.OK || ev.Link.Hash == "" {
		t.Fatalf("evidence: %+v", ev)
	}
}

func TestPh8_TamperDetected(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	r1, _ := store.Ingest([]byte("aaa"), "a")
	_, _ = store.Ingest([]byte("bbb"), "b")
	_, _ = store.Ingest([]byte("ccc"), "c")

	// Tamper stored original of first row.
	if _, err := store.db.Exec(`UPDATE invoices SET original = ? WHERE id = ?`, []byte("TAMPERED"), r1.ID); err != nil {
		t.Fatal(err)
	}
	rep, err := store.VerifyChain()
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("expected broken chain")
	}
	if rep.FirstBrokenID != r1.ID {
		t.Fatalf("first_broken_id=%d want %d", rep.FirstBrokenID, r1.ID)
	}
}

func TestPh8_Backfill(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pre.db")

	// Simulate a pre-Phase-8 DB: invoices without chain_links.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE invoices (
  id INTEGER PRIMARY KEY, external_id TEXT UNIQUE, format TEXT NOT NULL, status TEXT NOT NULL,
  vendor_name TEXT, invoice_number TEXT, buyer_name TEXT, invoice_date TEXT,
  total REAL, vat_amount REAL, currency TEXT, report_json TEXT, doc_gobl_json TEXT NOT NULL,
  original BLOB, created_at TEXT NOT NULL, source TEXT
);
INSERT INTO invoices (external_id, format, status, doc_gobl_json, original, created_at)
VALUES ('e1','unknown','parse_error','{}',X'61','2026-01-01T00:00:00Z'),
       ('e2','unknown','parse_error','{}',X'62','2026-01-02T00:00:00Z');
`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM chain_links`).Scan(&n)
	if n != 2 {
		t.Fatalf("backfill links=%d want 2", n)
	}
	rep, err := store.VerifyChain()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.LinkCount != 2 {
		t.Fatalf("after backfill: %+v", rep)
	}
}

func TestPh8_EvidenceNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.EvidenceFor(999)
	if err == nil {
		t.Fatal("expected not found")
	}
}
