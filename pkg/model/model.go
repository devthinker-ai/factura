// Package model is a thin view layer over GOBL invoices.
//
// Invariant: the GOBL JSON envelope (or its canonical fields) is the system of
// record. This package never invents a parallel schema.
package model

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/addons/de/xrechnung"
	"github.com/invopop/gobl/addons/de/zugferd"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/tax"

	// Register GOBL schemas, DE regime, XRechnung + ZUGFeRD addons.
	_ "github.com/invopop/gobl"
	_ "github.com/invopop/gobl/addons/de/zugferd"
	_ "github.com/invopop/gobl/regimes/de"
)

// Invoice wraps a GOBL envelope whose document is a bill.Invoice.
type Invoice struct {
	env *gobl.Envelope
}

// VATBucket is one VAT rate line from the invoice totals.
type VATBucket struct {
	Rate    string // e.g. "19%" / "7%" / "reverse-charge"
	Base    int64  // cents
	VAT     int64  // cents
	Percent string
}

// NewDEB2B builds a minimal valid German B2B invoice with mixed 19% + 7% lines.
// Tax IDs are the GOBL-documented sample DE numbers (mod-11 valid).
//
// Spike pin: bill.Invoice + tax.WithAddons(xrechnung.V3) + gobl.Envelop /
// env.Calculate — see Spike.md §a.
func NewDEB2B(code string, issueDate cal.Date) (*Invoice, error) {
	if code == "" {
		code = "R-2026-001"
	}
	if !issueDate.IsValid() {
		issueDate = cal.MakeDate(2026, 3, 1)
	}
	inv := &bill.Invoice{
		Regime:    tax.WithRegime("DE"),
		// Both addons: XR for issue CII, ZUGFeRD for hybrid PDF context.
		Addons: tax.WithAddons(xrechnung.V3, zugferd.V2),
		Type:      bill.InvoiceTypeStandard,
		Currency:  currency.EUR,
		IssueDate: issueDate,
		Code:      cbc.Code(code),
		Supplier: &org.Party{
			Name: "Provide One GmbH",
			TaxID: &tax.Identity{
				Country: "DE",
				Code:    "111111125",
			},
			Inboxes: []*org.Inbox{{Email: "billing@example.com"}},
			People: []*org.Person{{
				Name: &org.Name{Given: "Ada", Surname: "Lovelace"},
				Emails: []*org.Email{{
					Address: "billing@example.com",
				}},
				Telephones: []*org.Telephone{{Number: "+49100200300"}},
			}},
			Addresses: []*org.Address{{
				Street:   "Dietmar-Hopp-Allee",
				Number:   "16",
				Locality: "Walldorf",
				Code:     "69190",
				Country:  "DE",
			}},
			Telephones: []*org.Telephone{{Number: "+49100200300"}},
			Emails:     []*org.Email{{Address: "billing@example.com"}},
		},
		Customer: &org.Party{
			Name: "Sample Consumer",
			TaxID: &tax.Identity{
				Country: "DE",
				Code:    "282741168",
			},
			Inboxes: []*org.Inbox{{Email: "customer@example.com"}},
			Addresses: []*org.Address{{
				Street:   "Werner-Heisenberg-Allee",
				Number:   "25",
				Locality: "München",
				Code:     "80939",
				Country:  "DE",
			}},
		},
		Lines: []*bill.Line{
			{
				Quantity: num.MakeAmount(1, 0),
				Item: &org.Item{
					Name:  "Consulting",
					Price: num.NewAmount(10000, 2), // €100.00
					Unit:  "h",
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}},
			},
			{
				Quantity: num.MakeAmount(2, 0),
				Item: &org.Item{
					Name:  "Books",
					Price: num.NewAmount(2000, 2), // €20.00 each
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateReduced}},
			},
		},
		Ordering: &bill.Ordering{Code: "PO-42"}, // BR-DE-15 buyer reference
		Payment: &bill.PaymentDetails{
			Instructions: &pay.Instructions{
				Key: "credit-transfer+sepa",
				CreditTransfer: []*pay.CreditTransfer{{
					IBAN: "DE89370400440532013000",
				}},
			},
			Terms: &pay.Terms{Notes: "14 Tage netto"},
		},
	}
	return FromBill(inv)
}

// FromBill envelops, calculates, and wraps a bill.Invoice.
func FromBill(inv *bill.Invoice) (*Invoice, error) {
	if inv == nil {
		return nil, fmt.Errorf("model: nil invoice")
	}
	env, err := gobl.Envelop(inv)
	if err != nil {
		return nil, fmt.Errorf("model: envelop: %w", err)
	}
	if err := env.Calculate(); err != nil {
		return nil, fmt.Errorf("model: calculate: %w", err)
	}
	return &Invoice{env: env}, nil
}

// FromEnvelope wraps an existing calculated GOBL envelope.
func FromEnvelope(env *gobl.Envelope) (*Invoice, error) {
	if env == nil {
		return nil, fmt.Errorf("model: nil envelope")
	}
	if _, err := billFromEnv(env); err != nil {
		return nil, err
	}
	return &Invoice{env: env}, nil
}

// FromJSON parses a GOBL envelope or bare bill.Invoice JSON document.
func FromJSON(data []byte) (*Invoice, error) {
	obj, err := gobl.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("model: parse json: %w", err)
	}
	switch v := obj.(type) {
	case *gobl.Envelope:
		if err := v.Calculate(); err != nil {
			return nil, fmt.Errorf("model: calculate: %w", err)
		}
		return FromEnvelope(v)
	case *bill.Invoice:
		return FromBill(v)
	default:
		return nil, fmt.Errorf("model: unsupported gobl type %T", obj)
	}
}

// Envelope returns the underlying GOBL envelope (system of record).
func (i *Invoice) Envelope() *gobl.Envelope {
	if i == nil {
		return nil
	}
	return i.env
}

// Bill returns the embedded bill.Invoice.
func (i *Invoice) Bill() *bill.Invoice {
	if i == nil || i.env == nil {
		return nil
	}
	b, _ := billFromEnv(i.env)
	return b
}

// JSON returns the GOBL envelope as JSON (archive SoR blob).
func (i *Invoice) JSON() ([]byte, error) {
	if i == nil || i.env == nil {
		return nil, fmt.Errorf("model: nil invoice")
	}
	return json.MarshalIndent(i.env, "", "  ")
}

// VendorName is the supplier party name.
func (i *Invoice) VendorName() string {
	b := i.Bill()
	if b == nil || b.Supplier == nil {
		return ""
	}
	return b.Supplier.Name
}

// BuyerName is the customer party name.
func (i *Invoice) BuyerName() string {
	b := i.Bill()
	if b == nil || b.Customer == nil {
		return ""
	}
	return b.Customer.Name
}

// InvoiceNumber is series-code or code alone.
func (i *Invoice) InvoiceNumber() string {
	b := i.Bill()
	if b == nil {
		return ""
	}
	if b.Series != "" {
		return string(b.Series) + "-" + string(b.Code)
	}
	return string(b.Code)
}

// InvoiceDate returns the issue date as YYYY-MM-DD, or empty.
func (i *Invoice) InvoiceDate() string {
	b := i.Bill()
	if b == nil || !b.IssueDate.IsValid() {
		return ""
	}
	return b.IssueDate.String()
}

// Currency code (e.g. EUR).
func (i *Invoice) Currency() string {
	b := i.Bill()
	if b == nil {
		return ""
	}
	return string(b.Currency)
}

// TotalCents is payable total in integer cents (REAL only at SQLite edge).
func (i *Invoice) TotalCents() int64 {
	b := i.Bill()
	if b == nil || b.Totals == nil {
		return 0
	}
	return amountToCents(b.Totals.Payable)
}

// VATCents is total VAT in integer cents.
func (i *Invoice) VATCents() int64 {
	b := i.Bill()
	if b == nil || b.Totals == nil {
		return 0
	}
	return amountToCents(b.Totals.Tax)
}

// VATBreakdown returns per-rate VAT buckets in cents.
func (i *Invoice) VATBreakdown() []VATBucket {
	b := i.Bill()
	if b == nil || b.Totals == nil || b.Totals.Taxes == nil {
		return nil
	}
	var out []VATBucket
	for _, cat := range b.Totals.Taxes.Categories {
		if cat.Code != tax.CategoryVAT {
			continue
		}
		for _, rate := range cat.Rates {
			bucket := VATBucket{Rate: string(rate.Key)}
			if rate.Percent != nil {
				bucket.Percent = rate.Percent.String()
				bucket.Rate = rate.Percent.String()
			}
			if rate.Key == tax.KeyReverseCharge {
				bucket.Rate = "reverse-charge"
			}
			bucket.Base = amountToCents(rate.Base)
			bucket.VAT = amountToCents(rate.Amount)
			out = append(out, bucket)
		}
	}
	return out
}

// SellerTaxID returns the supplier VAT ID (country+code), empty if missing.
func (i *Invoice) SellerTaxID() string {
	b := i.Bill()
	if b == nil || b.Supplier == nil || b.Supplier.TaxID == nil {
		return ""
	}
	return string(b.Supplier.TaxID.Country) + string(b.Supplier.TaxID.Code)
}

// BuyerTaxID returns the customer VAT ID (country+code), empty if missing.
func (i *Invoice) BuyerTaxID() string {
	b := i.Bill()
	if b == nil || b.Customer == nil || b.Customer.TaxID == nil {
		return ""
	}
	return string(b.Customer.TaxID.Country) + string(b.Customer.TaxID.Code)
}

func billFromEnv(env *gobl.Envelope) (*bill.Invoice, error) {
	doc := env.Extract()
	if doc == nil {
		return nil, fmt.Errorf("model: empty envelope document")
	}
	inv, ok := doc.(*bill.Invoice)
	if !ok {
		return nil, fmt.Errorf("model: envelope doc is %T, want *bill.Invoice", doc)
	}
	return inv, nil
}

// amountToCents rescales to 2 decimal places and returns the integer value.
// Money is integer cents everywhere except the SQLite REAL edge.
func amountToCents(a num.Amount) int64 {
	r := a.Rescale(2)
	return r.Value()
}

// CentsToFloat converts integer cents to a float64 euro amount for SQLite REAL.
func CentsToFloat(cents int64) float64 {
	return float64(cents) / 100.0
}
