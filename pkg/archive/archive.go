// Package archive stores every ingested e-invoice in SQLite (modernc.org/sqlite,
// CGO_ENABLED=0). Invalid and parse_error rows are kept — never silently dropped.
//
// Money: integer cents in Go; REAL only at the SQLite edge (see model.CentsToFloat).
package archive

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// Status values persisted on each row.
const (
	StatusValid      = "valid"
	StatusInvalid    = "invalid"
	StatusParseError = "parse_error"
)

// ErrDuplicate is returned when external_id already exists.
var ErrDuplicate = errors.New("archive: external_id already exists")

// ErrNotFound is returned when a row id does not exist.
var ErrNotFound = errors.New("archive: not found")

// Store is a SQLite-backed invoice archive.
type Store struct {
	db *sql.DB
}

// Row is one archived invoice.
// JSON tags are the Phase 1/2 API contract — frozen.
type Row struct {
	ID            int64           `json:"id"`
	ExternalID    string          `json:"external_id"`
	Format        string          `json:"format"`
	Status        string          `json:"status"`
	VendorName    string          `json:"vendor_name"`
	InvoiceNumber string          `json:"invoice_number"`
	BuyerName     string          `json:"buyer_name"`
	InvoiceDate   string          `json:"invoice_date"`
	Total         float64         `json:"total"`
	VATAmount     float64         `json:"vat_amount"`
	Currency      string          `json:"currency"`
	Report        json.RawMessage `json:"report_json,omitempty"`
	DocGOBL       json.RawMessage `json:"doc_gobl_json,omitempty"`
	CreatedAt     string          `json:"created_at"`
	Source        string          `json:"source,omitempty"`
	InvoiceType   string          `json:"invoice_type,omitempty"` // BT-3: 380/381/384/389
}

// Filter selects rows for List / Search / Export / Count.
type Filter struct {
	Status string
	Vendor string
	Query  string // LIKE on vendor or invoice_number
	Since  string // YYYY-MM-DD
	Until  string // YYYY-MM-DD
	Limit  int    // 0 = no limit (List default applied by API)
	Offset int
}

// Open opens (or creates) the archive database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultDBPath()
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("archive: open: %w", err)
	}
	// Busy timeout helps concurrent API ingest vs unique-check races.
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("archive: pragma: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// DefaultDBPath returns FACTURA_DB or ./factura.db.
func DefaultDBPath() string {
	if v := os.Getenv("FACTURA_DB"); v != "" {
		return v
	}
	return "./factura.db"
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS invoices (
  id            INTEGER PRIMARY KEY,
  external_id   TEXT UNIQUE,
  format        TEXT NOT NULL,
  status        TEXT NOT NULL,
  vendor_name   TEXT,
  invoice_number TEXT,
  buyer_name    TEXT,
  invoice_date  TEXT,
  total         REAL,
  vat_amount    REAL,
  currency      TEXT,
  report_json   TEXT,
  doc_gobl_json TEXT NOT NULL,
  original      BLOB,
  created_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_invoices_vendor ON invoices(vendor_name);
CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number);
CREATE INDEX IF NOT EXISTS idx_invoices_date   ON invoices(invoice_date);
`)
	if err != nil {
		return err
	}
	// Phase 2: additive source column (idempotent for Phase 1 DBs).
	if err := s.ensureColumn("invoices", "source", "TEXT"); err != nil {
		return err
	}
	// Phase 4: Peppol participants + GoBD network-leg messages (invoices untouched).
	if err := s.migratePeppol(); err != nil {
		return err
	}
	// Phase 6: companies, api_keys, settings (additive; existing tables untouched).
	if err := s.migratePhase6(); err != nil {
		return err
	}
	// Phase 7: console users + roles + TOTP MFA.
	if err := s.migratePhase7(); err != nil {
		return err
	}
	// Phase 8: invoice_type column + GoBD append-only hash chain.
	if err := s.migratePhase8(); err != nil {
		return err
	}
	// Phase 11: per-company branding_json (logo / accent / header / footer).
	return s.migratePhase11()
}

func (s *Store) ensureColumn(table, col, decl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + decl)
	return err
}

// Ingest parses, validates, and always stores the input (even on parse_error).
func (s *Store) Ingest(data []byte, idHint string) (Row, error) {
	return s.IngestWithSource(data, idHint, "")
}

// IngestWithSource is Ingest plus an optional source tag (API header / batch/watch).
func (s *Store) IngestWithSource(data []byte, idHint, source string) (Row, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := Row{
		Format:    string(parse.FormatUnknown),
		Status:    StatusParseError,
		CreatedAt: now,
		DocGOBL:   json.RawMessage(`{}`),
		Source:    source,
	}

	inv, format, parseErr := parse.Parse(data)
	row.Format = string(format)

	if parseErr != nil {
		row.Status = StatusParseError
		rep := validate.Report{
			Valid:   false,
			Format:  string(format),
			Level:   "schema",
			Summary: parseErr.Error(),
			Violations: []validate.Violation{{
				RuleID: "XML-PARSE",
				Human:  parseErr.Error(),
			}},
		}
		repJSON, _ := json.Marshal(rep)
		row.Report = repJSON
		extID := idHint
		if extID == "" {
			extID = hashID("parse_error", string(format), data)
		}
		row.ExternalID = extID
		if existing, err := s.GetByExternalID(extID); err == nil {
			return existing, fmt.Errorf("%w: %s", ErrDuplicate, extID)
		}
		id, err := s.insert(row, data)
		if err != nil {
			if isUniqueViolation(err) {
				if existing, gerr := s.GetByExternalID(extID); gerr == nil {
					return existing, fmt.Errorf("%w: %s", ErrDuplicate, extID)
				}
			}
			return Row{}, err
		}
		row.ID = id
		return row, nil
	}

	row.VendorName = inv.VendorName()
	row.BuyerName = inv.BuyerName()
	row.InvoiceNumber = inv.InvoiceNumber()
	row.InvoiceDate = inv.InvoiceDate()
	row.Currency = inv.Currency()
	row.Total = model.CentsToFloat(inv.TotalCents())
	row.VATAmount = model.CentsToFloat(inv.VATCents())
	row.InvoiceType = inv.TypeCode()
	goblJSON, err := inv.JSON()
	if err != nil {
		goblJSON = []byte(`{}`)
	}
	row.DocGOBL = goblJSON

	extID := idHint
	if extID == "" {
		extID = hashID(row.VendorName, row.InvoiceNumber, row.InvoiceDate)
	}
	row.ExternalID = extID

	if existing, err := s.GetByExternalID(extID); err == nil {
		return existing, fmt.Errorf("%w: %s", ErrDuplicate, extID)
	}

	// WHY ValidateBytes: validate the original payload, not a GOBL recalculation.
	rep := validate.ValidateBytes(data)
	repJSON, _ := json.Marshal(rep)
	row.Report = repJSON
	if rep.Valid {
		row.Status = StatusValid
	} else {
		row.Status = StatusInvalid
	}

	id, err := s.insert(row, data)
	if err != nil {
		if isUniqueViolation(err) {
			if existing, gerr := s.GetByExternalID(extID); gerr == nil {
				return existing, fmt.Errorf("%w: %s", ErrDuplicate, extID)
			}
		}
		return Row{}, err
	}
	row.ID = id
	return row, nil
}

func (s *Store) insert(row Row, original []byte) (int64, error) {
	return s.insertWithChain(row, original)
}

// List returns rows matching filter (empty filter = all), newest first.
func (s *Store) List(f Filter) ([]Row, error) {
	return s.query(f, false)
}

// Count returns the number of rows matching filter (ignores Limit/Offset).
func (s *Store) Count(f Filter) (int, error) {
	where, args := filterWhere(f)
	sqlStr := `SELECT COUNT(*) FROM invoices`
	if where != "" {
		sqlStr += " WHERE " + where
	}
	var n int
	err := s.db.QueryRow(sqlStr, args...).Scan(&n)
	return n, err
}

// Search is List with a free-text query on vendor/number.
func (s *Store) Search(query, since, until, status, vendor string) ([]Row, error) {
	return s.query(Filter{
		Query:  query,
		Since:  since,
		Until:  until,
		Status: status,
		Vendor: vendor,
	}, false)
}

// Get returns one row by id (includes report + gobl JSON).
func (s *Store) Get(id int64) (Row, error) {
	row, err := s.scanFullRow(`
SELECT id, external_id, format, status, IFNULL(vendor_name,''), IFNULL(invoice_number,''),
  IFNULL(buyer_name,''), IFNULL(invoice_date,''), IFNULL(total,0), IFNULL(vat_amount,0),
  IFNULL(currency,''), report_json, doc_gobl_json, created_at, IFNULL(source,''), IFNULL(invoice_type,'')
FROM invoices WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	return row, err
}

// GetByExternalID returns the row for a unique external_id.
func (s *Store) GetByExternalID(externalID string) (Row, error) {
	row, err := s.scanFullRow(`
SELECT id, external_id, format, status, IFNULL(vendor_name,''), IFNULL(invoice_number,''),
  IFNULL(buyer_name,''), IFNULL(invoice_date,''), IFNULL(total,0), IFNULL(vat_amount,0),
  IFNULL(currency,''), report_json, doc_gobl_json, created_at, IFNULL(source,''), IFNULL(invoice_type,'')
FROM invoices WHERE external_id = ?`, externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, fmt.Errorf("%w: %s", ErrNotFound, externalID)
	}
	return row, err
}

func (s *Store) scanFullRow(q string, arg any) (Row, error) {
	row := Row{}
	var report, gobl sql.NullString
	err := s.db.QueryRow(q, arg).Scan(
		&row.ID, &row.ExternalID, &row.Format, &row.Status, &row.VendorName, &row.InvoiceNumber,
		&row.BuyerName, &row.InvoiceDate, &row.Total, &row.VATAmount, &row.Currency,
		&report, &gobl, &row.CreatedAt, &row.Source, &row.InvoiceType,
	)
	if err != nil {
		return Row{}, err
	}
	if report.Valid {
		row.Report = json.RawMessage(report.String)
	}
	if gobl.Valid {
		row.DocGOBL = json.RawMessage(gobl.String)
	}
	return row, nil
}

// Original returns the original ingested bytes.
func (s *Store) Original(id int64) ([]byte, error) {
	var blob []byte
	err := s.db.QueryRow(`SELECT original FROM invoices WHERE id = ?`, id).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return blob, nil
}

// DistinctVendors returns unique non-empty vendor names, sorted.
// vendor_name is already indexed (Phase 1). DISTINCT on a small self-hosted
// archive is fine; an index-only covering query or a cache would only matter
// once the archive grows past tens of thousands of distinct vendors.
func (s *Store) DistinctVendors(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.Query(`
SELECT DISTINCT vendor_name FROM invoices
WHERE vendor_name IS NOT NULL AND vendor_name <> ''
ORDER BY vendor_name
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	if out == nil {
		out = []string{}
	}
	return out, rows.Err()
}

// Export writes RFC 4180 CSV of filtered rows.
// Header ends with `invoice_type` (Phase 8 additive — after Phase 2 `source`).
func (s *Store) Export(f Filter, w io.Writer) error {
	rows, err := s.query(f, true)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "external_id", "format", "status", "vendor", "invoice_number",
		"invoice_date", "total", "vat", "currency", "source", "invoice_type",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			fmt.Sprintf("%d", r.ID),
			r.ExternalID,
			r.Format,
			r.Status,
			r.VendorName,
			r.InvoiceNumber,
			r.InvoiceDate,
			fmt.Sprintf("%.2f", r.Total),
			fmt.Sprintf("%.2f", r.VATAmount),
			r.Currency,
			r.Source,
			r.InvoiceType,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func filterWhere(f Filter) (string, []any) {
	var where []string
	var args []any
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Vendor != "" {
		where = append(where, "vendor_name LIKE ?")
		args = append(args, "%"+f.Vendor+"%")
	}
	if f.Query != "" {
		where = append(where, "(vendor_name LIKE ? OR invoice_number LIKE ?)")
		q := "%" + f.Query + "%"
		args = append(args, q, q)
	}
	if f.Since != "" {
		where = append(where, "invoice_date >= ?")
		args = append(args, f.Since)
	}
	if f.Until != "" {
		where = append(where, "invoice_date <= ?")
		args = append(args, f.Until)
	}
	return strings.Join(where, " AND "), args
}

func (s *Store) query(f Filter, forExport bool) ([]Row, error) {
	_ = forExport
	where, args := filterWhere(f)
	sqlStr := `
SELECT id, external_id, format, status, IFNULL(vendor_name,''), IFNULL(invoice_number,''),
  IFNULL(buyer_name,''), IFNULL(invoice_date,''), IFNULL(total,0), IFNULL(vat_amount,0),
  IFNULL(currency,''), report_json, created_at, IFNULL(source,''), IFNULL(invoice_type,'')
FROM invoices`
	if where != "" {
		sqlStr += " WHERE " + where
	}
	sqlStr += " ORDER BY id DESC"
	if f.Limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", f.Limit)
		if f.Offset > 0 {
			sqlStr += fmt.Sprintf(" OFFSET %d", f.Offset)
		}
	} else if f.Offset > 0 {
		sqlStr += fmt.Sprintf(" LIMIT -1 OFFSET %d", f.Offset)
	}

	rs, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var out []Row
	for rs.Next() {
		var row Row
		var report sql.NullString
		if err := rs.Scan(
			&row.ID, &row.ExternalID, &row.Format, &row.Status, &row.VendorName, &row.InvoiceNumber,
			&row.BuyerName, &row.InvoiceDate, &row.Total, &row.VATAmount, &row.Currency,
			&report, &row.CreatedAt, &row.Source, &row.InvoiceType,
		); err != nil {
			return nil, err
		}
		if report.Valid {
			row.Report = json.RawMessage(report.String)
		}
		out = append(out, row)
	}
	return out, rs.Err()
}

func hashID(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%v\x00", p)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
