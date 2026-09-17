package archive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"
)

// GoBD hash-chain genesis (prev_hash of the first link). Documented in AGENTS.md.
const chainGenesisSeed = "factura-gobd-chain-v1"

// ChainLink is one append-only GoBD evidence seal over an invoice row.
type ChainLink struct {
	ID        int64  `json:"id"`
	InvoiceID int64  `json:"invoice_id"`
	PrevHash  string `json:"prev_hash"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"created_at"`
}

// ChainReport is the result of VerifyChain / GET /audit.
type ChainReport struct {
	OK            bool   `json:"ok"`
	LinkCount     int    `json:"link_count"`
	Head          string `json:"head"`
	FirstBrokenID int64  `json:"first_broken_id"` // 0 when ok
	ExpectedHash  string `json:"expected_hash,omitempty"`
	ActualHash    string `json:"actual_hash,omitempty"`
}

// EvidenceBundle is the per-invoice evidence view.
type EvidenceBundle struct {
	Link ChainLink  `json:"link"`
	Prev *ChainLink `json:"prev,omitempty"`
	Next *ChainLink `json:"next,omitempty"`
	OK   bool       `json:"ok"` // this link consistent with its predecessor
}

func (s *Store) migratePhase8() error {
	if err := s.ensureColumn("invoices", "invoice_type", "TEXT"); err != nil {
		return err
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS chain_links (
  id         INTEGER PRIMARY KEY,
  invoice_id INTEGER NOT NULL,
  prev_hash  TEXT NOT NULL,
  hash       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chain_links_invoice ON chain_links(invoice_id);
`)
	if err != nil {
		return err
	}
	return s.backfillChain()
}

func (s *Store) backfillChain() error {
	var nLinks, nInvoices int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM chain_links`).Scan(&nLinks); err != nil {
		return err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM invoices`).Scan(&nInvoices); err != nil {
		return err
	}
	if nLinks > 0 || nInvoices == 0 {
		return nil
	}
	log.Printf("archive: GoBD chain backfill — sealing %d pre-Phase-8 rows (proves integrity only from this upgrade)", nInvoices)

	// Load all rows first — SetMaxOpenConns(1) cannot hold a Query open while BeginTx.
	type rowSnap struct {
		id                        int64
		extID, format, status, at string
		original, gobl, report    []byte
	}
	rs, err := s.db.Query(`
SELECT id, IFNULL(external_id,''), IFNULL(format,''), IFNULL(status,''), created_at,
  IFNULL(original, X''), IFNULL(doc_gobl_json,''), IFNULL(report_json,'')
FROM invoices ORDER BY id ASC`)
	if err != nil {
		return err
	}
	var snaps []rowSnap
	for rs.Next() {
		var r rowSnap
		if err := rs.Scan(&r.id, &r.extID, &r.format, &r.status, &r.at, &r.original, &r.gobl, &r.report); err != nil {
			rs.Close()
			return err
		}
		snaps = append(snaps, r)
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return err
	}
	rs.Close()

	prev := chainGenesis()
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range snaps {
		digest := chainDigest(r.id, r.extID, r.format, r.status, r.at, r.original, r.gobl, r.report)
		h := chainLinkHash(prev, digest)
		if _, err := tx.Exec(`
INSERT INTO chain_links (id, invoice_id, prev_hash, hash, created_at)
VALUES (?, ?, ?, ?, ?)`, r.id, r.id, prev, h, now); err != nil {
			return err
		}
		prev = h
	}
	return tx.Commit()
}

func chainGenesis() string {
	sum := sha256.Sum256([]byte(chainGenesisSeed))
	return hex.EncodeToString(sum[:])
}

func blobHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// chainDigest = sha256(id || \x00 || external_id || … || blobhash(report_json))
func chainDigest(id int64, externalID, format, status, createdAt string, original, gobl, report []byte) []byte {
	h := sha256.New()
	h.Write([]byte(strconv.FormatInt(id, 10)))
	h.Write([]byte{0})
	h.Write([]byte(externalID))
	h.Write([]byte{0})
	h.Write([]byte(format))
	h.Write([]byte{0})
	h.Write([]byte(status))
	h.Write([]byte{0})
	h.Write([]byte(createdAt))
	h.Write([]byte{0})
	h.Write([]byte(blobHash(original)))
	h.Write([]byte{0})
	h.Write([]byte(blobHash(gobl)))
	h.Write([]byte{0})
	h.Write([]byte(blobHash(report)))
	return h.Sum(nil)
}

func chainLinkHash(prevHex string, digest []byte) string {
	h := sha256.New()
	h.Write([]byte(prevHex))
	h.Write(digest)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) latestChainHash(tx *sql.Tx) (string, error) {
	var h string
	err := tx.QueryRow(`SELECT hash FROM chain_links ORDER BY id DESC LIMIT 1`).Scan(&h)
	if err == sql.ErrNoRows {
		return chainGenesis(), nil
	}
	return h, err
}

// insertWithChain writes the invoice row and its chain link in one transaction.
func (s *Store) insertWithChain(row Row, original []byte) (int64, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, fmt.Errorf("archive: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
INSERT INTO invoices (
  external_id, format, status, vendor_name, invoice_number, buyer_name,
  invoice_date, total, vat_amount, currency, report_json, doc_gobl_json, original,
  created_at, source, invoice_type
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ExternalID, row.Format, row.Status, nullStr(row.VendorName), nullStr(row.InvoiceNumber),
		nullStr(row.BuyerName), nullStr(row.InvoiceDate), row.Total, row.VATAmount, nullStr(row.Currency),
		string(row.Report), string(row.DocGOBL), original, row.CreatedAt, nullStr(row.Source),
		nullStr(row.InvoiceType),
	)
	if err != nil {
		return 0, fmt.Errorf("archive: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	prev, err := s.latestChainHash(tx)
	if err != nil {
		return 0, fmt.Errorf("archive: chain head: %w", err)
	}
	digest := chainDigest(id, row.ExternalID, row.Format, row.Status, row.CreatedAt,
		original, []byte(row.DocGOBL), []byte(row.Report))
	h := chainLinkHash(prev, digest)
	now := row.CreatedAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := tx.Exec(`
INSERT INTO chain_links (id, invoice_id, prev_hash, hash, created_at)
VALUES (?, ?, ?, ?, ?)`, id, id, prev, h, now); err != nil {
		return 0, fmt.Errorf("archive: chain link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// VerifyChain recomputes genesis→head and reports the first broken link.
func (s *Store) VerifyChain() (ChainReport, error) {
	rows, err := s.db.Query(`
SELECT cl.id, cl.invoice_id, cl.prev_hash, cl.hash, cl.created_at,
  IFNULL(i.external_id,''), IFNULL(i.format,''), IFNULL(i.status,''), i.created_at,
  IFNULL(i.original, X''), IFNULL(i.doc_gobl_json,''), IFNULL(i.report_json,'')
FROM chain_links cl
JOIN invoices i ON i.id = cl.invoice_id
ORDER BY cl.id ASC`)
	if err != nil {
		return ChainReport{}, err
	}
	defer rows.Close()

	prev := chainGenesis()
	var count int
	var head string
	for rows.Next() {
		var link ChainLink
		var extID, format, status, invCreated string
		var original, gobl, report []byte
		if err := rows.Scan(
			&link.ID, &link.InvoiceID, &link.PrevHash, &link.Hash, &link.CreatedAt,
			&extID, &format, &status, &invCreated, &original, &gobl, &report,
		); err != nil {
			return ChainReport{}, err
		}
		count++
		if link.PrevHash != prev {
			return ChainReport{
				OK: false, LinkCount: count, Head: head, FirstBrokenID: link.InvoiceID,
				ExpectedHash: prev, ActualHash: link.PrevHash,
			}, nil
		}
		digest := chainDigest(link.InvoiceID, extID, format, status, invCreated, original, gobl, report)
		want := chainLinkHash(prev, digest)
		if want != link.Hash {
			return ChainReport{
				OK: false, LinkCount: count, Head: head, FirstBrokenID: link.InvoiceID,
				ExpectedHash: want, ActualHash: link.Hash,
			}, nil
		}
		prev = link.Hash
		head = link.Hash
	}
	if err := rows.Err(); err != nil {
		return ChainReport{}, err
	}
	return ChainReport{OK: true, LinkCount: count, Head: head, FirstBrokenID: 0}, nil
}

// ChainHead returns the latest chain link, or ErrNotFound when empty.
func (s *Store) ChainHead() (ChainLink, error) {
	var l ChainLink
	err := s.db.QueryRow(`
SELECT id, invoice_id, prev_hash, hash, created_at FROM chain_links
ORDER BY id DESC LIMIT 1`).Scan(&l.ID, &l.InvoiceID, &l.PrevHash, &l.Hash, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return ChainLink{}, fmt.Errorf("%w: chain empty", ErrNotFound)
	}
	return l, err
}

// EvidenceFor returns the chain link for an invoice plus neighbours.
func (s *Store) EvidenceFor(invoiceID int64) (EvidenceBundle, error) {
	var l ChainLink
	err := s.db.QueryRow(`
SELECT id, invoice_id, prev_hash, hash, created_at FROM chain_links WHERE invoice_id = ?`, invoiceID).
		Scan(&l.ID, &l.InvoiceID, &l.PrevHash, &l.Hash, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return EvidenceBundle{}, fmt.Errorf("%w: %d", ErrNotFound, invoiceID)
	}
	if err != nil {
		return EvidenceBundle{}, err
	}

	out := EvidenceBundle{Link: l}
	var prev ChainLink
	perr := s.db.QueryRow(`
SELECT id, invoice_id, prev_hash, hash, created_at FROM chain_links
WHERE id < ? ORDER BY id DESC LIMIT 1`, l.ID).
		Scan(&prev.ID, &prev.InvoiceID, &prev.PrevHash, &prev.Hash, &prev.CreatedAt)
	if perr == nil {
		out.Prev = &prev
	} else if perr != sql.ErrNoRows {
		return EvidenceBundle{}, perr
	}
	var next ChainLink
	nerr := s.db.QueryRow(`
SELECT id, invoice_id, prev_hash, hash, created_at FROM chain_links
WHERE id > ? ORDER BY id ASC LIMIT 1`, l.ID).
		Scan(&next.ID, &next.InvoiceID, &next.PrevHash, &next.Hash, &next.CreatedAt)
	if nerr == nil {
		out.Next = &next
	} else if nerr != sql.ErrNoRows {
		return EvidenceBundle{}, nerr
	}

	// Spot-check: recompute this link against its predecessor.
	var extID, format, status, invCreated string
	var original, gobl, report []byte
	if err := s.db.QueryRow(`
SELECT IFNULL(external_id,''), IFNULL(format,''), IFNULL(status,''), created_at,
  IFNULL(original, X''), IFNULL(doc_gobl_json,''), IFNULL(report_json,'')
FROM invoices WHERE id = ?`, invoiceID).Scan(&extID, &format, &status, &invCreated, &original, &gobl, &report); err != nil {
		return EvidenceBundle{}, err
	}
	prevHash := chainGenesis()
	if out.Prev != nil {
		prevHash = out.Prev.Hash
	}
	want := chainLinkHash(prevHash, chainDigest(invoiceID, extID, format, status, invCreated, original, gobl, report))
	out.OK = want == l.Hash && l.PrevHash == prevHash
	return out, nil
}
