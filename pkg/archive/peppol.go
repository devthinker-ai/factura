package archive

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Peppol message status / direction (mirror peppol package constants for DB).
const (
	PeppolDirIn  = "in"
	PeppolDirOut = "out"

	PeppolSending   = "sending"
	PeppolAccepted  = "accepted"
	PeppolDelivered = "delivered"
	PeppolFailed    = "failed"
)

// Participant is a Peppol-identified business stored in SQLite.
type Participant struct {
	PeppolID           string `json:"peppol_id"`
	ServiceType        string `json:"service_type"`
	Name               string `json:"name"`
	IsSelf             bool   `json:"is_self"`
	RegisteredEndpoint string `json:"registered_endpoint,omitempty"`
	CreatedAt          string `json:"created_at"`
}

// PeppolMessage is the GoBD network-leg record (BIS + receipt blobs).
type PeppolMessage struct {
	ID           int64  `json:"id"`
	Direction    string `json:"direction"`
	FromPeppolID string `json:"from_peppol_id"`
	ToPeppolID   string `json:"to_peppol_id"`
	APMessageID  string `json:"ap_message_id"`
	Status       string `json:"status"`
	BisXML       []byte `json:"-"`
	Receipt      []byte `json:"-"`
	InvoiceID    *int64 `json:"invoice_id,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	// Optional decoded strings for API (populated by handlers).
	BisXMLText  string `json:"bis_xml,omitempty"`
	ReceiptText string `json:"receipt,omitempty"`
}

func (s *Store) migratePeppol() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS participants (
  peppol_id TEXT PRIMARY KEY,
  service_type TEXT NOT NULL,
  name TEXT,
  is_self INTEGER NOT NULL DEFAULT 0,
  registered_endpoint TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS peppol_messages (
  id INTEGER PRIMARY KEY,
  direction TEXT NOT NULL,
  from_peppol_id TEXT,
  to_peppol_id TEXT,
  ap_message_id TEXT,
  status TEXT NOT NULL,
  bis_xml BLOB,
  receipt BLOB,
  invoice_id INTEGER,
  external_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_peppol_ap_message_id
  ON peppol_messages(ap_message_id) WHERE ap_message_id IS NOT NULL AND ap_message_id <> '';
CREATE INDEX IF NOT EXISTS idx_peppol_direction ON peppol_messages(direction);
CREATE INDEX IF NOT EXISTS idx_peppol_status ON peppol_messages(status);
`)
	return err
}

// Participants returns all registered Peppol participants.
func (s *Store) Participants() ([]Participant, error) {
	rows, err := s.db.Query(`
SELECT peppol_id, service_type, IFNULL(name,''), is_self, IFNULL(registered_endpoint,''), created_at
FROM participants ORDER BY is_self DESC, peppol_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Participant
	for rows.Next() {
		var p Participant
		var self int
		if err := rows.Scan(&p.PeppolID, &p.ServiceType, &p.Name, &self, &p.RegisteredEndpoint, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.IsSelf = self == 1
		out = append(out, p)
	}
	if out == nil {
		out = []Participant{}
	}
	return out, rows.Err()
}

// AddParticipant inserts or replaces a participant.
func (s *Store) AddParticipant(p Participant) error {
	if strings.TrimSpace(p.PeppolID) == "" {
		return fmt.Errorf("archive: peppol_id required")
	}
	if p.ServiceType == "" {
		p.ServiceType = "buyer-seller"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	self := 0
	if p.IsSelf {
		self = 1
		// Clear previous self flags so Self() is unambiguous.
		if _, err := s.db.Exec(`UPDATE participants SET is_self = 0 WHERE is_self = 1`); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`
INSERT INTO participants (peppol_id, service_type, name, is_self, registered_endpoint, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(peppol_id) DO UPDATE SET
  service_type = excluded.service_type,
  name = excluded.name,
  is_self = excluded.is_self,
  registered_endpoint = excluded.registered_endpoint
`, p.PeppolID, p.ServiceType, nullStr(p.Name), self, nullStr(p.RegisteredEndpoint), p.CreatedAt)
	return err
}

// Self returns the is_self=1 participant.
func (s *Store) Self() (Participant, error) {
	var p Participant
	var self int
	err := s.db.QueryRow(`
SELECT peppol_id, service_type, IFNULL(name,''), is_self, IFNULL(registered_endpoint,''), created_at
FROM participants WHERE is_self = 1 LIMIT 1`).Scan(
		&p.PeppolID, &p.ServiceType, &p.Name, &self, &p.RegisteredEndpoint, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Participant{}, fmt.Errorf("%w: self participant", ErrNotFound)
	}
	if err != nil {
		return Participant{}, err
	}
	p.IsSelf = true
	return p, nil
}

// GetParticipant returns one participant by Peppol ID.
func (s *Store) GetParticipant(peppolID string) (Participant, error) {
	var p Participant
	var self int
	err := s.db.QueryRow(`
SELECT peppol_id, service_type, IFNULL(name,''), is_self, IFNULL(registered_endpoint,''), created_at
FROM participants WHERE peppol_id = ?`, peppolID).Scan(
		&p.PeppolID, &p.ServiceType, &p.Name, &self, &p.RegisteredEndpoint, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Participant{}, fmt.Errorf("%w: %s", ErrNotFound, peppolID)
	}
	if err != nil {
		return Participant{}, err
	}
	p.IsSelf = self == 1
	return p, nil
}

// SendRecord creates a peppol_messages row in status sending.
func (s *Store) SendRecord(dir, from, to string, bis []byte, externalID string, invoiceID *int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
INSERT INTO peppol_messages (
  direction, from_peppol_id, to_peppol_id, ap_message_id, status, bis_xml, receipt,
  invoice_id, external_id, created_at, updated_at
) VALUES (?, ?, ?, '', ?, ?, NULL, ?, ?, ?, ?)`,
		dir, from, to, PeppolSending, bis, invoiceID, nullStr(externalID), now, now)
	if err != nil {
		return 0, fmt.Errorf("archive: send record: %w", err)
	}
	return res.LastInsertId()
}

// RecordInbound inserts an inbound message row (or returns existing by ap_message_id).
func (s *Store) RecordInbound(from, to, apMessageID, status string, bis []byte, invoiceID *int64, externalID string) (PeppolMessage, bool, error) {
	if apMessageID != "" {
		if existing, err := s.MessageByAPMessageID(apMessageID); err == nil {
			return existing, true, nil
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if status == "" {
		status = PeppolDelivered
	}
	res, err := s.db.Exec(`
INSERT INTO peppol_messages (
  direction, from_peppol_id, to_peppol_id, ap_message_id, status, bis_xml, receipt,
  invoice_id, external_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
		PeppolDirIn, from, to, apMessageID, status, bis, invoiceID, nullStr(externalID), now, now)
	if err != nil {
		if isUniqueViolation(err) && apMessageID != "" {
			existing, gerr := s.MessageByAPMessageID(apMessageID)
			if gerr == nil {
				return existing, true, nil
			}
		}
		return PeppolMessage{}, false, err
	}
	id, _ := res.LastInsertId()
	msg, err := s.MessageByID(id)
	return msg, false, err
}

// MarkDelivered sets status delivered|accepted and archives the receipt blob.
func (s *Store) MarkDelivered(msgID int64, receipt []byte) error {
	return s.markStatus(msgID, PeppolDelivered, receipt, nil)
}

// MarkAccepted sets status accepted with receipt.
func (s *Store) MarkAccepted(msgID int64, receipt []byte) error {
	return s.markStatus(msgID, PeppolAccepted, receipt, nil)
}

// MarkFailed sets status failed with optional receipt.
func (s *Store) MarkFailed(msgID int64, receipt []byte) error {
	return s.markStatus(msgID, PeppolFailed, receipt, nil)
}

// UpdatePeppolMessage updates ap_message_id, status, receipt, invoice link.
func (s *Store) UpdatePeppolMessage(msgID int64, apMessageID, status string, receipt []byte, invoiceID *int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
UPDATE peppol_messages SET
  ap_message_id = COALESCE(NULLIF(?, ''), ap_message_id),
  status = COALESCE(NULLIF(?, ''), status),
  receipt = COALESCE(?, receipt),
  invoice_id = COALESCE(?, invoice_id),
  updated_at = ?
WHERE id = ?`, apMessageID, status, receipt, invoiceID, now, msgID)
	return err
}

func (s *Store) markStatus(msgID int64, status string, receipt []byte, invoiceID *int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
UPDATE peppol_messages SET status = ?, receipt = ?, updated_at = ?,
  invoice_id = COALESCE(?, invoice_id)
WHERE id = ?`, status, receipt, now, invoiceID, msgID)
	return err
}

// SetPeppolInvoiceID links a message to an invoice row.
func (s *Store) SetPeppolInvoiceID(msgID, invoiceID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE peppol_messages SET invoice_id = ?, updated_at = ? WHERE id = ?`,
		invoiceID, now, msgID)
	return err
}

// SetPeppolAPMessageID sets the AP idempotency key after send.
func (s *Store) SetPeppolAPMessageID(msgID int64, apMessageID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE peppol_messages SET ap_message_id = ?, updated_at = ? WHERE id = ?`,
		apMessageID, now, msgID)
	return err
}

// MessageByID returns one peppol_messages row.
func (s *Store) MessageByID(id int64) (PeppolMessage, error) {
	m, err := s.scanPeppolMessage(`
SELECT id, direction, IFNULL(from_peppol_id,''), IFNULL(to_peppol_id,''), IFNULL(ap_message_id,''),
  status, bis_xml, receipt, invoice_id, IFNULL(external_id,''), created_at, updated_at
FROM peppol_messages WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return PeppolMessage{}, fmt.Errorf("%w: peppol message %d", ErrNotFound, id)
	}
	return m, err
}

// MessageByAPMessageID looks up by AP idempotency key.
func (s *Store) MessageByAPMessageID(apMessageID string) (PeppolMessage, error) {
	m, err := s.scanPeppolMessage(`
SELECT id, direction, IFNULL(from_peppol_id,''), IFNULL(to_peppol_id,''), IFNULL(ap_message_id,''),
  status, bis_xml, receipt, invoice_id, IFNULL(external_id,''), created_at, updated_at
FROM peppol_messages WHERE ap_message_id = ?`, apMessageID)
	if errors.Is(err, sql.ErrNoRows) {
		return PeppolMessage{}, fmt.Errorf("%w: ap_message_id %s", ErrNotFound, apMessageID)
	}
	return m, err
}

// ListMessages returns peppol_messages filtered by direction/status.
func (s *Store) ListMessages(dir, status, since string, limit int) ([]PeppolMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var where []string
	var args []any
	if dir != "" {
		where = append(where, "direction = ?")
		args = append(args, dir)
	}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if since != "" {
		where = append(where, "created_at >= ?")
		args = append(args, since)
	}
	sqlStr := `
SELECT id, direction, IFNULL(from_peppol_id,''), IFNULL(to_peppol_id,''), IFNULL(ap_message_id,''),
  status, bis_xml, receipt, invoice_id, IFNULL(external_id,''), created_at, updated_at
FROM peppol_messages`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += fmt.Sprintf(" ORDER BY id DESC LIMIT %d", limit)
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeppolMessage
	for rows.Next() {
		m, err := scanPeppolRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []PeppolMessage{}
	}
	return out, rows.Err()
}

// CountMessagesByStatus returns counts keyed by status (optional direction filter).
func (s *Store) CountMessagesByStatus(dir string) (map[string]int, error) {
	sqlStr := `SELECT status, COUNT(*) FROM peppol_messages`
	var args []any
	if dir != "" {
		sqlStr += ` WHERE direction = ?`
		args = append(args, dir)
	}
	sqlStr += ` GROUP BY status`
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

func (s *Store) scanPeppolMessage(q string, arg any) (PeppolMessage, error) {
	row := s.db.QueryRow(q, arg)
	return scanPeppolRow(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPeppolRow(sc scanner) (PeppolMessage, error) {
	var m PeppolMessage
	var inv sql.NullInt64
	var bis, rec []byte
	err := sc.Scan(
		&m.ID, &m.Direction, &m.FromPeppolID, &m.ToPeppolID, &m.APMessageID,
		&m.Status, &bis, &rec, &inv, &m.ExternalID, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return PeppolMessage{}, err
	}
	m.BisXML = bis
	m.Receipt = rec
	if inv.Valid {
		id := inv.Int64
		m.InvoiceID = &id
	}
	return m, nil
}
