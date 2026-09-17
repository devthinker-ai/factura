package model

import (
	"encoding/base64"
	"fmt"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/tax"
)

// UNTDID 1001 type codes (BT-3) — the read-side mapping for archive/API.
const (
	TypeCodeStandard   = "380"
	TypeCodeCreditNote = "381"
	TypeCodeCorrective = "384"
	TypeCodeSelfBilled = "389"
)

// AttachmentSpec is the builder input for BT-125 embedded supporting documents.
// Bytes live inside the GOBL document as a data-URI — no file path.
type AttachmentSpec struct {
	Name        string
	Description string
	MIME        string
	Data        []byte
}

// TypeCode returns the UNTDID 1001 BT-3 code for this invoice.
// Mapping: standard→380, credit-note→381, corrective→384, self-billed tag→389.
// Self-billed wins over Type when tax.TagSelfBilled is set (GOBL expresses 389
// as a tag on a standard invoice, not a Type value).
func (i *Invoice) TypeCode() string {
	b := i.Bill()
	if b == nil {
		return ""
	}
	if b.HasTags(tax.TagSelfBilled) {
		return TypeCodeSelfBilled
	}
	switch b.Type {
	case bill.InvoiceTypeCreditNote:
		return TypeCodeCreditNote
	case bill.InvoiceTypeCorrective:
		return TypeCodeCorrective
	case bill.InvoiceTypeStandard, cbc.KeyEmpty:
		return TypeCodeStandard
	default:
		return ""
	}
}

// NewCreditNote builds a DE B2B credit note (BT-3 = 381) with a BG-3 preceding
// reference. Amounts follow the signed convention: line prices are negated so
// totals are negative — see AGENTS.md "Credit-note sign convention".
func NewCreditNote(baseCode string, issueDate cal.Date, precedingCode string, precedingDate cal.Date) (*Invoice, error) {
	inv, err := NewDEB2B(baseCode, issueDate)
	if err != nil {
		return nil, err
	}
	b := inv.Bill()
	b.Type = bill.InvoiceTypeCreditNote
	b.Preceding = []*org.DocumentRef{precedingRef(precedingCode, precedingDate)}
	for _, line := range b.Lines {
		if line != nil && line.Item != nil && line.Item.Price != nil {
			neg := line.Item.Price.Negate()
			line.Item.Price = &neg
		}
	}
	return FromBill(b)
}

// NewCorrected builds a DE B2B corrected invoice (BT-3 = 384) with a BG-3
// preceding reference.
func NewCorrected(baseCode string, issueDate cal.Date, precedingCode string, precedingDate cal.Date) (*Invoice, error) {
	inv, err := NewDEB2B(baseCode, issueDate)
	if err != nil {
		return nil, err
	}
	b := inv.Bill()
	b.Type = bill.InvoiceTypeCorrective
	b.Preceding = []*org.DocumentRef{precedingRef(precedingCode, precedingDate)}
	return FromBill(b)
}

// NewSelfBilled builds a DE B2B self-billed invoice (BT-3 = 389).
// Canonical GOBL field: tax.TagSelfBilled via SetTags (serializes as $tags).
func NewSelfBilled(baseCode string, issueDate cal.Date) (*Invoice, error) {
	inv, err := NewDEB2B(baseCode, issueDate)
	if err != nil {
		return nil, err
	}
	b := inv.Bill()
	b.Type = bill.InvoiceTypeStandard
	b.SetTags(tax.TagSelfBilled)
	return FromBill(b)
}

// WithAttachments appends BT-125 supporting documents as data-URI attachments.
// MIME defaults to application/pdf when empty. Returns a recalculated Invoice.
func WithAttachments(inv *Invoice, specs []AttachmentSpec) (*Invoice, error) {
	if inv == nil {
		return nil, fmt.Errorf("model: nil invoice")
	}
	b := inv.Bill()
	if b == nil {
		return nil, fmt.Errorf("model: empty bill")
	}
	for i, s := range specs {
		mime := s.MIME
		if mime == "" {
			mime = "application/pdf"
		}
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("attachment-%d", i+1)
		}
		url := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(s.Data)
		b.Attachments = append(b.Attachments, &org.Attachment{
			Code:        cbc.Code(fmt.Sprintf("ATT-%d", len(b.Attachments)+1)),
			Name:        name,
			Description: s.Description,
			MIME:        mime,
			URL:         url,
		})
	}
	return FromBill(b)
}

// WithLeitwegID sets the buyer reference (BT-10 / BR-DE-15) — the B2G Leitweg-ID
// slot on bill.Ordering.Code → CII ram:BuyerReference.
func WithLeitwegID(inv *Invoice, id string) (*Invoice, error) {
	if inv == nil {
		return nil, fmt.Errorf("model: nil invoice")
	}
	b := inv.Bill()
	if b == nil {
		return nil, fmt.Errorf("model: empty bill")
	}
	if b.Ordering == nil {
		b.Ordering = &bill.Ordering{}
	}
	b.Ordering.Code = cbc.Code(id)
	return FromBill(b)
}

func precedingRef(code string, issueDate cal.Date) *org.DocumentRef {
	d := issueDate
	return &org.DocumentRef{
		Code:      cbc.Code(code),
		IssueDate: &d,
	}
}
