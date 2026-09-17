package archive

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrDuplicateEmail is returned when email already exists.
var ErrDuplicateEmail = errors.New("archive: duplicate email")

// Role values for console users.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// ValidRole reports whether role is admin|editor|viewer.
func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleEditor, RoleViewer:
		return true
	}
	return false
}

// User is a console account. List/Get never expose hashes or TOTP secrets.
type User struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Role            string `json:"role"`
	TOTPEnabled     bool   `json:"totp_enabled"`
	TOTPConfirmedAt string `json:"totp_confirmed_at,omitempty"`
	CreatedAt       string `json:"created_at"`

	// Secrets — never serialized in list/DTO responses.
	PasswordHash  string   `json:"-"`
	TOTPSecret    string   `json:"-"`
	RecoveryCodes []string `json:"-"` // sha256 hex digests
}

func (s *Store) migratePhase7() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('admin','editor','viewer')),
  password_hash TEXT NOT NULL,
  totp_secret TEXT NOT NULL DEFAULT '',
  totp_enabled INTEGER NOT NULL DEFAULT 0,
  totp_confirmed_at TEXT,
  recovery_codes TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL
);
`)
	return err
}

func newUserID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CountUsers returns the number of rows in users.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// AddUser inserts a user. passwordHash must already be argon2id PHC.
func (s *Store) AddUser(email, name, role, passwordHash string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	role = strings.TrimSpace(role)
	if email == "" {
		return User{}, fmt.Errorf("archive: email required")
	}
	if !ValidRole(role) {
		return User{}, fmt.Errorf("archive: invalid role")
	}
	if passwordHash == "" {
		return User{}, fmt.Errorf("archive: password hash required")
	}
	id, err := newUserID()
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
INSERT INTO users(id, email, name, role, password_hash, totp_secret, totp_enabled, totp_confirmed_at, recovery_codes, created_at)
VALUES(?,?,?,?,?,'',0,NULL,'[]',?)`, id, email, name, role, passwordHash, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrDuplicateEmail
		}
		return User{}, err
	}
	return User{
		ID: id, Email: email, Name: name, Role: role,
		CreatedAt: now, PasswordHash: passwordHash,
	}, nil
}

// ListUsers returns all users without hashes/secrets.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
SELECT id, email, name, role, totp_enabled, COALESCE(totp_confirmed_at,''), created_at
FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var en int
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &en, &u.TOTPConfirmedAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.TOTPEnabled = en == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserByEmail returns the full user row (incl. secrets) or ErrNotFound.
func (s *Store) UserByEmail(email string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	return s.scanUser(`SELECT id, email, name, role, password_hash, totp_secret, totp_enabled,
COALESCE(totp_confirmed_at,''), recovery_codes, created_at FROM users WHERE email = ?`, email)
}

// GetUser returns the full user row by id or ErrNotFound.
func (s *Store) GetUser(id string) (User, error) {
	return s.scanUser(`SELECT id, email, name, role, password_hash, totp_secret, totp_enabled,
COALESCE(totp_confirmed_at,''), recovery_codes, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) scanUser(q string, arg any) (User, error) {
	var u User
	var en int
	var codes string
	err := s.db.QueryRow(q, arg).Scan(
		&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.TOTPSecret,
		&en, &u.TOTPConfirmedAt, &codes, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.TOTPEnabled = en == 1
	_ = json.Unmarshal([]byte(codes), &u.RecoveryCodes)
	if u.RecoveryCodes == nil {
		u.RecoveryCodes = []string{}
	}
	return u, nil
}

// SetPassword updates password_hash.
func (s *Store) SetPassword(id, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveUser deletes a user by id.
func (s *Store) RemoveUser(id string) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRole updates the role.
func (s *Store) SetRole(id, role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("archive: invalid role")
	}
	res, err := s.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertTOTP stores secret/enabled/codes/confirmedAt. codes are sha256 hex digests.
func (s *Store) UpsertTOTP(id, secret string, enabled bool, codes []string, confirmedAt string) error {
	if codes == nil {
		codes = []string{}
	}
	raw, err := json.Marshal(codes)
	if err != nil {
		return err
	}
	en := 0
	if enabled {
		en = 1
	}
	var conf any
	if confirmedAt == "" {
		conf = nil
	} else {
		conf = confirmedAt
	}
	res, err := s.db.Exec(`
UPDATE users SET totp_secret = ?, totp_enabled = ?, recovery_codes = ?, totp_confirmed_at = ?
WHERE id = ?`, secret, en, string(raw), conf, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ConsumeRecoveryCode removes codeHash from recovery_codes if present.
func (s *Store) ConsumeRecoveryCode(userID, codeHash string) (bool, error) {
	u, err := s.GetUser(userID)
	if err != nil {
		return false, err
	}
	found := -1
	for i, h := range u.RecoveryCodes {
		if h == codeHash {
			found = i
			break
		}
	}
	if found < 0 {
		return false, nil
	}
	u.RecoveryCodes = append(u.RecoveryCodes[:found], u.RecoveryCodes[found+1:]...)
	raw, err := json.Marshal(u.RecoveryCodes)
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`UPDATE users SET recovery_codes = ? WHERE id = ?`, string(raw), userID)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ClearTOTP disables 2FA and clears secret + recovery codes (idempotent).
func (s *Store) ClearTOTP(id string) error {
	_, err := s.db.Exec(`
UPDATE users SET totp_secret = '', totp_enabled = 0, totp_confirmed_at = NULL, recovery_codes = '[]'
WHERE id = ?`, id)
	return err
}
