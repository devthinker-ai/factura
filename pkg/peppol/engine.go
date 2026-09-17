package peppol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/generate"
	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// ErrBadEnvelope means the body is not parseable BIS 3.0 SBD.
var ErrBadEnvelope = errors.New("peppol: bad envelope")

// ErrValidation is returned when our own validator rejects an outgoing invoice.
var ErrValidation = errors.New("peppol: validation failed")

// ErrAPRejected means the AP returned a failed/rejected receipt status.
var ErrAPRejected = errors.New("peppol: ap rejected")

// ErrAPUnreachable means Send failed after retries.
var ErrAPUnreachable = errors.New("peppol: ap unreachable")

// Engine wires Store + AccessPoint for receive/send (API + CLI).
type Engine struct {
	Store  *archive.Store
	AP     AccessPoint
	SelfID string
}

// ReceiveResult is the outcome of handling one inbound SBD.
type ReceiveResult struct {
	InvoiceID  int64  `json:"invoice_id"`
	Status     string `json:"status"`
	ExternalID string `json:"external_id"`
	MessageID  int64  `json:"message_id"`
	Duplicate  bool   `json:"duplicate,omitempty"`
}

// ReceiveSBD unwraps BIS 3.0 → Ingest → archives the network leg.
func (e *Engine) ReceiveSBD(raw []byte) (ReceiveResult, error) {
	msg, err := FromXML(raw)
	if err != nil {
		m, _, _ := e.Store.RecordInbound("", "", "", archive.PeppolFailed, raw, nil, "")
		return ReceiveResult{MessageID: m.ID, Status: archive.PeppolFailed}, fmt.Errorf("%w: %v", ErrBadEnvelope, err)
	}

	apID := msg.Control.APMessageID
	if apID != "" {
		if existing, err := e.Store.MessageByAPMessageID(apID); err == nil {
			invID := int64(0)
			st := ""
			if existing.InvoiceID != nil {
				invID = *existing.InvoiceID
				if row, gerr := e.Store.Get(invID); gerr == nil {
					st = row.Status
				}
			}
			return ReceiveResult{
				InvoiceID:  invID,
				Status:     st,
				ExternalID: existing.ExternalID,
				MessageID:  existing.ID,
				Duplicate:  true,
			}, nil
		}
	}

	extID := ExternalIDFromMessage(msg)
	source := "peppol:receive"
	selfID := e.selfID()
	// Own sends bouncing back (sender == self) are distinguishable.
	if selfID != "" && msg.Sender.Identifier == selfID {
		source = "peppol:receive:self"
	}

	row, err := e.Store.IngestWithSource(msg.Payload, extID, source)
	if err != nil && row.ID == 0 {
		return ReceiveResult{}, err
	}
	invID := row.ID
	status := archive.PeppolDelivered
	if msg.Control.Status == StatusFailed {
		status = archive.PeppolFailed
	}
	m, dup, err := e.Store.RecordInbound(
		msg.Sender.Identifier, msg.Receiver.Identifier, apID, status, raw, &invID, extID)
	if err != nil {
		return ReceiveResult{}, err
	}
	return ReceiveResult{
		InvoiceID:  invID,
		Status:     row.Status,
		ExternalID: row.ExternalID,
		MessageID:  m.ID,
		Duplicate:  dup,
	}, nil
}

func (e *Engine) selfID() string {
	if e.SelfID != "" {
		return e.SelfID
	}
	if p, err := e.Store.Self(); err == nil {
		return p.PeppolID
	}
	return os.Getenv("FACTURA_PEPPOL_SELF_ID")
}

// PollAndIngest pulls from the AP and receives each message.
func (e *Engine) PollAndIngest(ctx context.Context, since time.Time) ([]ReceiveResult, error) {
	if e.AP == nil {
		return nil, fmt.Errorf("peppol: no access point")
	}
	msgs, err := e.AP.Poll(ctx, since)
	if err != nil {
		return nil, err
	}
	var out []ReceiveResult
	for _, m := range msgs {
		raw, err := m.ToXML()
		if err != nil {
			continue
		}
		res, rerr := e.ReceiveSBD(raw)
		if rerr != nil && res.MessageID == 0 {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// SendRequest is the JSON body for POST /peppol/send.
type SendRequest struct {
	InvoiceID int64           `json:"invoice_id"`
	To        string          `json:"to"`
	GOBL      json.RawMessage `json:"gobl"`
}

// SendResult is returned on successful delivery (or partial failure with IDs).
type SendResult struct {
	MessageID int64          `json:"message_id"`
	Status    string         `json:"status"`
	InvoiceID int64          `json:"invoice_id"`
	Receipt   map[string]any `json:"receipt,omitempty"`
	Report    *validate.Report `json:"report,omitempty"`
	Detail    string         `json:"detail,omitempty"`
}

// Send delivers an invoice via the Access Point.
func (e *Engine) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	if e.AP == nil {
		return SendResult{}, fmt.Errorf("peppol: no access point")
	}
	to := req.To
	if to == "" {
		return SendResult{}, fmt.Errorf("%w: missing to", archive.ErrNotFound)
	}
	if _, err := e.Store.GetParticipant(to); err != nil {
		return SendResult{}, fmt.Errorf("%w: participant %s", archive.ErrNotFound, to)
	}

	selfID := e.selfID()
	if selfID == "" {
		return SendResult{}, fmt.Errorf("peppol: FACTURA_PEPPOL_SELF_ID / self participant required")
	}

	payload, invoiceID, extID, err := e.resolvePayload(req)
	if err != nil {
		return SendResult{}, err
	}

	rep := validate.ValidateBytes(payload)
	if !rep.Valid {
		return SendResult{Report: &rep}, fmt.Errorf("%w: %s", ErrValidation, rep.Summary)
	}

	apMsgID := fmt.Sprintf("factura-out-%d", time.Now().UTC().UnixNano())
	bis := BisMessage{
		CollaborationProtocol: CollaborationProtocol,
		Process:               ProcessURI,
		Sender:                PartyID{Identifier: selfID},
		Receiver:              PartyID{Identifier: to},
		Payload:               payload,
		PayloadMime:           PayloadMimeXML,
		Control: Control{
			Sender:       selfID,
			Receiver:     to,
			CreationTime: time.Now().UTC(),
			APMessageID:  apMsgID,
		},
	}
	bisXML, err := bis.ToXML()
	if err != nil {
		return SendResult{}, err
	}

	var invPtr *int64
	if invoiceID > 0 {
		invPtr = &invoiceID
	}
	msgID, err := e.Store.SendRecord(archive.PeppolDirOut, selfID, to, bisXML, extID, invPtr)
	if err != nil {
		return SendResult{}, err
	}
	_ = e.Store.SetPeppolAPMessageID(msgID, apMsgID)

	rec, err := SendWithRetry(ctx, e.AP, bis)
	if err != nil {
		_ = e.Store.MarkFailed(msgID, []byte(err.Error()))
		return SendResult{MessageID: msgID, Status: archive.PeppolFailed, InvoiceID: invoiceID},
			fmt.Errorf("%w: %v", ErrAPUnreachable, err)
	}

	if rec.Status == StatusFailed {
		_ = e.Store.MarkFailed(msgID, rec.Raw)
		_ = e.Store.SetPeppolAPMessageID(msgID, rec.APMessageID)
		return SendResult{
			MessageID: msgID,
			Status:    archive.PeppolFailed,
			InvoiceID: invoiceID,
			Detail:    rec.Status,
			Receipt: map[string]any{
				"ap_message_id": rec.APMessageID,
				"status":        rec.Status,
				"at":            rec.At.UTC().Format(time.RFC3339),
			},
		}, fmt.Errorf("%w: %s", ErrAPRejected, rec.Status)
	}

	status := archive.PeppolDelivered
	if rec.Status == StatusAccepted {
		status = archive.PeppolAccepted
		_ = e.Store.MarkAccepted(msgID, rec.Raw)
	} else {
		_ = e.Store.MarkDelivered(msgID, rec.Raw)
	}
	_ = e.Store.SetPeppolAPMessageID(msgID, rec.APMessageID)

	// Archive outbound artifact with peppol:send source.
	sendExt := "peppol:send:" + rec.APMessageID
	row, ierr := e.Store.IngestWithSource(payload, sendExt, "peppol:send")
	if ierr == nil || row.ID > 0 {
		invoiceID = row.ID
		_ = e.Store.SetPeppolInvoiceID(msgID, row.ID)
	}

	return SendResult{
		MessageID: msgID,
		Status:    status,
		InvoiceID: invoiceID,
		Receipt: map[string]any{
			"ap_message_id": rec.APMessageID,
			"status":        rec.Status,
			"at":            rec.At.UTC().Format(time.RFC3339),
		},
	}, nil
}

// Retry re-sends a sending|failed outbound message.
func (e *Engine) Retry(ctx context.Context, messageID int64) (SendResult, error) {
	m, err := e.Store.MessageByID(messageID)
	if err != nil {
		return SendResult{}, err
	}
	if m.Direction != archive.PeppolDirOut {
		return SendResult{}, fmt.Errorf("%w: not outbound", archive.ErrNotFound)
	}
	if m.Status != archive.PeppolSending && m.Status != archive.PeppolFailed {
		return SendResult{}, fmt.Errorf("peppol: message status %s not retryable", m.Status)
	}
	bis, err := FromXML(m.BisXML)
	if err != nil {
		return SendResult{}, err
	}
	if bis.Control.APMessageID == "" {
		bis.Control.APMessageID = m.APMessageID
	}
	// Reset to sending.
	_ = e.Store.UpdatePeppolMessage(messageID, m.APMessageID, archive.PeppolSending, nil, m.InvoiceID)

	rec, err := SendWithRetry(ctx, e.AP, bis)
	invID := int64(0)
	if m.InvoiceID != nil {
		invID = *m.InvoiceID
	}
	if err != nil {
		_ = e.Store.MarkFailed(messageID, []byte(err.Error()))
		return SendResult{MessageID: messageID, Status: archive.PeppolFailed, InvoiceID: invID},
			fmt.Errorf("%w: %v", ErrAPUnreachable, err)
	}
	if rec.Status == StatusFailed {
		_ = e.Store.MarkFailed(messageID, rec.Raw)
		return SendResult{
			MessageID: messageID,
			Status:    archive.PeppolFailed,
			InvoiceID: invID,
			Detail:    rec.Status,
			Receipt: map[string]any{
				"ap_message_id": rec.APMessageID,
				"status":        rec.Status,
			},
		}, fmt.Errorf("%w: %s", ErrAPRejected, rec.Status)
	}
	status := archive.PeppolDelivered
	if rec.Status == StatusAccepted {
		status = archive.PeppolAccepted
		_ = e.Store.MarkAccepted(messageID, rec.Raw)
	} else {
		_ = e.Store.MarkDelivered(messageID, rec.Raw)
	}
	_ = e.Store.SetPeppolAPMessageID(messageID, rec.APMessageID)
	return SendResult{
		MessageID: messageID,
		Status:    status,
		InvoiceID: invID,
		Receipt: map[string]any{
			"ap_message_id": rec.APMessageID,
			"status":        rec.Status,
			"at":            rec.At.UTC().Format(time.RFC3339),
		},
	}, nil
}

func (e *Engine) resolvePayload(req SendRequest) (payload []byte, invoiceID int64, extID string, err error) {
	if len(req.GOBL) > 0 && string(req.GOBL) != "null" {
		inv, err := model.FromJSON(req.GOBL)
		if err != nil {
			return nil, 0, "", fmt.Errorf("peppol: gobl: %w", err)
		}
		dir, err := os.MkdirTemp("", "factura-peppol-*")
		if err != nil {
			return nil, 0, "", err
		}
		defer os.RemoveAll(dir)
		xrPath, _, err := generate.Generate(*inv, dir)
		if err != nil {
			return nil, 0, "", err
		}
		payload, err = os.ReadFile(xrPath)
		if err != nil {
			return nil, 0, "", err
		}
		extID = "peppol-gobl-" + inv.InvoiceNumber()
		return payload, 0, extID, nil
	}
	if req.InvoiceID < 1 {
		return nil, 0, "", fmt.Errorf("%w: invoice_id or gobl required", archive.ErrNotFound)
	}
	row, err := e.Store.Get(req.InvoiceID)
	if err != nil {
		return nil, 0, "", err
	}
	orig, err := e.Store.Original(req.InvoiceID)
	if err != nil {
		return nil, 0, "", err
	}
	// Prefer XML original; if PDF, regenerate from GOBL.
	if len(orig) > 0 && (orig[0] == '<' || (len(orig) > 5 && string(orig[:5]) == "<?xml")) {
		return orig, row.ID, row.ExternalID, nil
	}
	if len(row.DocGOBL) > 0 && string(row.DocGOBL) != "{}" {
		inv, err := model.FromJSON(row.DocGOBL)
		if err != nil {
			return nil, 0, "", err
		}
		dir, err := os.MkdirTemp("", "factura-peppol-*")
		if err != nil {
			return nil, 0, "", err
		}
		defer os.RemoveAll(dir)
		xrPath, _, err := generate.Generate(*inv, dir)
		if err != nil {
			return nil, 0, "", err
		}
		payload, err = os.ReadFile(xrPath)
		return payload, row.ID, row.ExternalID, err
	}
	return orig, row.ID, row.ExternalID, nil
}
