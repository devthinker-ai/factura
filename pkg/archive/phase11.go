package archive

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// CompanyBranding is per-company PDF presentation (logo, accent, header/footer).
// Stored as JSON in companies.branding_json. All fields optional.
type CompanyBranding struct {
	LogoB64    string `json:"logo_b64,omitempty"`
	LogoMime   string `json:"logo_mime,omitempty"`
	Accent     string `json:"accent,omitempty"`
	HeaderText string `json:"header_text,omitempty"`
	FooterText string `json:"footer_text,omitempty"`
}

func (s *Store) migratePhase11() error {
	return s.ensureColumn("companies", "branding_json", "TEXT")
}

// GetCompanyBranding returns branding for a company.
// Missing/NULL column → zero-value branding. ErrNotFound only when the company is absent.
func (s *Store) GetCompanyBranding(id int64) (*CompanyBranding, error) {
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT branding_json FROM companies WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := &CompanyBranding{}
	if !raw.Valid || raw.String == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw.String), out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetCompanyBranding marshals branding into companies.branding_json.
// nil clears the column (NULL). Returns ErrNotFound when the company is absent.
func (s *Store) SetCompanyBranding(id int64, b *CompanyBranding) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM companies WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if b == nil {
		_, err := s.db.Exec(`UPDATE companies SET branding_json = NULL WHERE id = ?`, id)
		return err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE companies SET branding_json = ? WHERE id = ?`, string(raw), id)
	return err
}
