package archive

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPlanLimit is returned when a tier cap would be exceeded (caller maps to 403).
var ErrPlanLimit = errors.New("archive: plan limit")

// ErrDuplicateVAT is returned when seller_vat_id already exists.
var ErrDuplicateVAT = errors.New("archive: duplicate seller_vat_id")

const (
	defaultKeyRPM     = 30
	defaultKeyMonthly = 1000
	keyPrefix         = "factura_"
	keyHexLen         = 24 // 192 bits
)

func (s *Store) migratePhase6() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS companies (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  seller_vat_id TEXT UNIQUE,
  peppol_id TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  key_hash TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  rpm INTEGER NOT NULL DEFAULT 30,
  monthly_invoices INTEGER NOT NULL DEFAULT 1000,
  invoices_this_month INTEGER NOT NULL DEFAULT 0,
  period TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`)
	return err
}

// ——— settings ———

// GetSetting returns a settings value or ("", ErrNotFound).
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting upserts a settings row.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
INSERT INTO settings(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, key, value)
	return err
}

// DeleteSetting removes a settings row. Returns ErrNotFound if absent.
func (s *Store) DeleteSetting(key string) error {
	res, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ——— companies ———

// Company is a registered seller identity (tier-cap anchor).
type Company struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	SellerVATID string `json:"seller_vat_id,omitempty"`
	PeppolID    string `json:"peppol_id,omitempty"`
	IsDefault   bool   `json:"is_default"`
	CreatedAt   string `json:"created_at"`
}

// ListCompanies returns all companies ordered by id.
func (s *Store) ListCompanies() ([]Company, error) {
	rows, err := s.db.Query(`
SELECT id, name, COALESCE(seller_vat_id,''), COALESCE(peppol_id,''), is_default, created_at
FROM companies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Company
	for rows.Next() {
		var c Company
		var def int
		if err := rows.Scan(&c.ID, &c.Name, &c.SellerVATID, &c.PeppolID, &def, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsDefault = def != 0
		out = append(out, c)
	}
	if out == nil {
		out = []Company{}
	}
	return out, rows.Err()
}

// AddCompany inserts a company. Empty name → error; duplicate VAT → ErrDuplicateVAT.
func (s *Store) AddCompany(name, vatID, peppolID string) (Company, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Company{}, fmt.Errorf("archive: company name required")
	}
	vatID = strings.TrimSpace(vatID)
	peppolID = strings.TrimSpace(peppolID)
	now := time.Now().UTC().Format(time.RFC3339)

	// First company becomes default automatically.
	existing, err := s.ListCompanies()
	if err != nil {
		return Company{}, err
	}
	isDefault := 0
	if len(existing) == 0 {
		isDefault = 1
	}

	var vat any
	if vatID != "" {
		vat = vatID
	}
	res, err := s.db.Exec(`
INSERT INTO companies(name, seller_vat_id, peppol_id, is_default, created_at)
VALUES(?,?,?,?,?)`, name, vat, nullIfEmpty(peppolID), isDefault, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return Company{}, ErrDuplicateVAT
		}
		return Company{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Company{}, err
	}
	return s.CompanyByID(id)
}

// CompanyByID returns one company or ErrNotFound.
func (s *Store) CompanyByID(id int64) (Company, error) {
	var c Company
	var def int
	var vat, pep sql.NullString
	err := s.db.QueryRow(`
SELECT id, name, seller_vat_id, peppol_id, is_default, created_at
FROM companies WHERE id = ?`, id).Scan(&c.ID, &c.Name, &vat, &pep, &def, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Company{}, ErrNotFound
	}
	if err != nil {
		return Company{}, err
	}
	c.SellerVATID = vat.String
	c.PeppolID = pep.String
	c.IsDefault = def != 0
	return c, nil
}

// SetDefaultCompany clears other defaults then sets id (transactional).
func (s *Store) SetDefaultCompany(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM companies WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(`UPDATE companies SET is_default = 0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE companies SET is_default = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DefaultCompany returns the default company, or ErrNotFound.
func (s *Store) DefaultCompany() (Company, error) {
	var c Company
	var def int
	var vat, pep sql.NullString
	err := s.db.QueryRow(`
SELECT id, name, seller_vat_id, peppol_id, is_default, created_at
FROM companies WHERE is_default = 1 LIMIT 1`).Scan(&c.ID, &c.Name, &vat, &pep, &def, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Company{}, ErrNotFound
	}
	if err != nil {
		return Company{}, err
	}
	c.SellerVATID = vat.String
	c.PeppolID = pep.String
	c.IsDefault = true
	return c, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ——— API keys ———

// APIKey is a machine key record (id = full factura_… string).
type APIKey struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	RPM                int    `json:"rpm"`
	MonthlyInvoices    int    `json:"monthly_invoices"`
	InvoicesThisMonth  int    `json:"invoices_this_month"`
	Period             string `json:"period,omitempty"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"created_at"`
	KeyHash            string `json:"-"`
}

// CreateKey mints a new key. rpm/monthly 0 → defaults (30 / 1000).
// Plaintext is returned in Key.ID exactly once at creation (caller must show it).
func (s *Store) CreateKey(name string, rpm, monthly int) (APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return APIKey{}, fmt.Errorf("archive: key name required")
	}
	if rpm <= 0 {
		rpm = defaultKeyRPM
	}
	if monthly <= 0 {
		monthly = defaultKeyMonthly
	}
	plain, err := mintKey()
	if err != nil {
		return APIKey{}, err
	}
	hash := HashAPIKey(plain)
	now := time.Now().UTC()
	period := now.Format("2006-01")
	created := now.Format(time.RFC3339)
	_, err = s.db.Exec(`
INSERT INTO api_keys(id, key_hash, name, rpm, monthly_invoices, invoices_this_month, period, enabled, created_at)
VALUES(?,?,?,?,?,0,?,1,?)`, plain, hash, name, rpm, monthly, period, created)
	if err != nil {
		return APIKey{}, err
	}
	return APIKey{
		ID:                plain,
		Name:              name,
		RPM:               rpm,
		MonthlyInvoices:   monthly,
		InvoicesThisMonth: 0,
		Period:            period,
		Enabled:           true,
		CreatedAt:         created,
		KeyHash:           hash,
	}, nil
}

// ListKeys returns all API keys (id = full key string).
func (s *Store) ListKeys() ([]APIKey, error) {
	rows, err := s.db.Query(`
SELECT id, key_hash, name, rpm, monthly_invoices, invoices_this_month, period, enabled, created_at
FROM api_keys ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	if out == nil {
		out = []APIKey{}
	}
	return out, rows.Err()
}

// KeyByID returns one key or ErrNotFound.
func (s *Store) KeyByID(id string) (APIKey, error) {
	row := s.db.QueryRow(`
SELECT id, key_hash, name, rpm, monthly_invoices, invoices_this_month, period, enabled, created_at
FROM api_keys WHERE id = ?`, id)
	return scanAPIKeyRow(row)
}

// KeyByHash looks up a key by sha256 hex.
func (s *Store) KeyByHash(hash string) (APIKey, error) {
	row := s.db.QueryRow(`
SELECT id, key_hash, name, rpm, monthly_invoices, invoices_this_month, period, enabled, created_at
FROM api_keys WHERE key_hash = ?`, hash)
	return scanAPIKeyRow(row)
}

// SetKeyEnabled toggles the kill switch.
func (s *Store) SetKeyEnabled(id string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE api_keys SET enabled = ? WHERE id = ?`, en, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKey hard-deletes a key.
func (s *Store) DeleteKey(id string) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementUsage bumps invoices_this_month for a key with calendar-month rollover.
// now is injectable for tests (nil → time.Now UTC).
func (s *Store) IncrementUsage(id string, now func() time.Time) (used int, err error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	period := now().Format("2006-01")
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var curPeriod string
	var cur int
	err = tx.QueryRow(`SELECT period, invoices_this_month FROM api_keys WHERE id = ?`, id).
		Scan(&curPeriod, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if curPeriod != period {
		cur = 0
		curPeriod = period
	}
	cur++
	_, err = tx.Exec(`UPDATE api_keys SET period = ?, invoices_this_month = ? WHERE id = ?`,
		curPeriod, cur, id)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return cur, nil
}

// IncrementOperatorUsage bumps the non-key invoice-ops counter (dashboard only).
func (s *Store) IncrementOperatorUsage(now func() time.Time) (used int, err error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	period := now().Format("2006-01")
	curPeriod, _ := s.GetSetting("operator_usage_period")
	cur := 0
	if curPeriod == period {
		if v, e := s.GetSetting("operator_usage_count"); e == nil {
			fmt.Sscanf(v, "%d", &cur)
		}
	}
	cur++
	if err := s.SetSetting("operator_usage_period", period); err != nil {
		return 0, err
	}
	if err := s.SetSetting("operator_usage_count", fmt.Sprintf("%d", cur)); err != nil {
		return 0, err
	}
	return cur, nil
}

// OperatorUsage returns the current calendar-month operator invoice-ops count.
func (s *Store) OperatorUsage(now func() time.Time) (period string, count int) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	period = now().Format("2006-01")
	curPeriod, err := s.GetSetting("operator_usage_period")
	if err != nil || curPeriod != period {
		return period, 0
	}
	if v, e := s.GetSetting("operator_usage_count"); e == nil {
		fmt.Sscanf(v, "%d", &count)
	}
	return period, count
}

// UsageSummary builds the GET /usage payload.
type KeyUsage struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	InvoicesThisMonth int    `json:"invoices_this_month"`
	Limit             int    `json:"limit"`
}

// UsageSummary is the aggregate metering view.
type UsageSummary struct {
	Period             string     `json:"period"`
	InvoicesThisMonth  int        `json:"invoices_this_month"`
	Keys               []KeyUsage `json:"keys"`
}

// UsageSummary returns operator + per-key usage for the current month.
func (s *Store) UsageSummary(now func() time.Time) (UsageSummary, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	period, opCount := s.OperatorUsage(now)
	keys, err := s.ListKeys()
	if err != nil {
		return UsageSummary{}, err
	}
	out := UsageSummary{
		Period:            period,
		InvoicesThisMonth: opCount,
		Keys:              make([]KeyUsage, 0, len(keys)),
	}
	for _, k := range keys {
		used := k.InvoicesThisMonth
		if k.Period != period {
			used = 0
		}
		out.Keys = append(out.Keys, KeyUsage{
			ID:                k.ID,
			Name:              k.Name,
			InvoicesThisMonth: used,
			Limit:             k.MonthlyInvoices,
		})
	}
	return out, nil
}

// HashAPIKey returns the SHA-256 hex digest of a plaintext key.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func mintKey() (string, error) {
	b := make([]byte, keyHexLen/2) // 12 bytes → 24 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return keyPrefix + hex.EncodeToString(b), nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAPIKey(rows *sql.Rows) (APIKey, error) {
	return scanAPIKeyRow(rows)
}

func scanAPIKeyRow(row scannable) (APIKey, error) {
	var k APIKey
	var en int
	err := row.Scan(&k.ID, &k.KeyHash, &k.Name, &k.RPM, &k.MonthlyInvoices,
		&k.InvoicesThisMonth, &k.Period, &en, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if err != nil {
		return APIKey{}, err
	}
	k.Enabled = en != 0
	return k, nil
}
