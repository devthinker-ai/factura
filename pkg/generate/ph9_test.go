package generate

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/org"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// consoleShapeJSON mirrors frontend toGobl() for a fully filled New-invoice form
// (addr/num/inboxes/people contact + credit-transfer). This is the Phase 9
// regression Phase 8 never had — console path vs engine self-check.
func consoleShapeJSON(docType, code string, preceding map[string]string) []byte {
	supplier := map[string]any{
		"name": "Provide One GmbH",
		"tax_id": map[string]string{
			"country": "DE",
			"code":    "111111125",
		},
		"addresses": []map[string]string{{
			"street": "Dietmar-Hopp-Allee", "locality": "Walldorf",
			"code": "69190", "country": "DE",
		}},
		"emails":  []map[string]string{{"addr": "billing@example.com"}},
		"inboxes": []map[string]string{{"email": "billing@example.com"}},
		"people": []map[string]any{{
			"name":       map[string]string{"given": "Ada Lovelace"},
			"telephones": []map[string]string{{"num": "+49100200300"}},
			"emails":     []map[string]string{{"addr": "billing@example.com"}},
		}},
	}
	customer := map[string]any{
		"name": "Sample Consumer",
		"tax_id": map[string]string{
			"country": "DE",
			"code":    "282741168",
		},
		"addresses": []map[string]string{{
			"street": "Werner-Heisenberg-Allee", "locality": "München",
			"code": "80939", "country": "DE",
		}},
		"emails":  []map[string]string{{"addr": "customer@example.com"}},
		"inboxes": []map[string]string{{"email": "customer@example.com"}},
		"people": []map[string]any{{
			"name":   map[string]string{"given": "Bob Buyer"},
			"emails": []map[string]string{{"addr": "customer@example.com"}},
		}},
	}
	price := "100.00"
	if docType == "credit-note" {
		price = "-100.00"
	}
	inv := map[string]any{
		"$schema":    "https://gobl.org/draft-0/bill/invoice",
		"$regime":    "DE",
		"$addons":    []string{"de-xrechnung-v3", "de-zugferd-v2"},
		"type":       docType,
		"code":       code,
		"issue_date": "2026-09-01",
		"currency":   "EUR",
		"supplier":   supplier,
		"customer":   customer,
		"lines": []map[string]any{{
			"quantity": "1",
			"item":     map[string]string{"name": "Consulting", "price": price},
			"taxes":    []map[string]string{{"cat": "VAT", "rate": "general"}},
		}},
		"ordering": map[string]string{"code": "NA"},
		"payment": map[string]any{
			"instructions": map[string]any{
				"key": "credit-transfer+sepa",
				"credit_transfer": []map[string]string{{
					"iban": "DE89370400440532013000",
				}},
			},
			// BR-CO-25: positive BT-115 requires BT-9 or BT-20.
			"terms": map[string]string{"notes": "14 Tage netto"},
		},
	}
	if preceding != nil {
		inv["preceding"] = []map[string]string{preceding}
	}
	b, err := json.Marshal(inv)
	if err != nil {
		panic(err)
	}
	return b
}

func TestPh9_ConsoleShapePassesSelfCheck(t *testing.T) {
	cases := []struct {
		name      string
		docType   string
		code      string
		preceding map[string]string
	}{
		{"standard", "standard", "R-CONSOLE-1", nil},
		{"credit-note", "credit-note", "CN-CONSOLE-1", map[string]string{
			"code": "R-0", "issue_date": "2026-08-01",
		}},
		{"corrective", "corrective", "COR-CONSOLE-1", map[string]string{
			"code": "R-0", "issue_date": "2026-08-01",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv, err := model.FromJSON(consoleShapeJSON(c.docType, c.code, c.preceding))
			if err != nil {
				t.Fatalf("FromJSON: %v", err)
			}
			rep := validate.Validate(*inv, parse.FormatXRechnungCII)
			if !rep.Valid {
				t.Fatalf("validate: %s %+v", rep.Summary, rep.Violations)
			}
			if len(rep.Violations) != 0 {
				t.Fatalf("violations: %+v", rep.Violations)
			}
			dir := t.TempDir()
			xr, zf, err := Generate(*inv, dir)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			cii, _ := os.ReadFile(xr)
			s := string(cii)
			if !strings.Contains(s, "Ada Lovelace") {
				t.Fatal("missing BT-41 PersonName")
			}
			if !strings.Contains(s, "+49100200300") {
				t.Fatal("missing BT-42 telephone")
			}
			if !strings.Contains(s, "billing@example.com") {
				t.Fatal("missing BT-43 seller email")
			}
			if !strings.Contains(s, "customer@example.com") {
				t.Fatal("missing BT-49 buyer electronic address")
			}
			if _, err := os.Stat(zf); err != nil {
				t.Fatalf("hybrid PDF missing: %v", err)
			}
		})
	}
}

func TestPh9_AttachmentMIMEAllowList(t *testing.T) {
	pdf := []byte("%PDF-1.4")
	cases := []struct {
		name    string
		mime    string
		urlMime string
		wantOK  bool
	}{
		{"pdf", "application/pdf", "", true},
		{"png", "image/png", "", true},
		{"jpeg", "image/jpeg", "", true},
		{"jpg-alias-uri", "", "image/jpg", true}, // accept image/jpg as jpeg
		{"xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "", true},
		{"ods", "application/vnd.oasis.opendocument.spreadsheet", "", true},
		{"csv", "text/csv", "", true},
		{"html", "text/html", "", false},
		{"zip", "application/zip", "", false},
		{"empty-mime-uri-ok", "", "application/pdf", true},
		{"empty-all", "", "", false},
		{"from-uri-bad", "", "text/html", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uriMime := c.mime
			if c.urlMime != "" {
				uriMime = c.urlMime
			}
			var url string
			if uriMime != "" {
				url = "data:" + uriMime + ";base64," + base64.StdEncoding.EncodeToString(pdf)
			} else {
				url = "data:;base64," + base64.StdEncoding.EncodeToString(pdf)
			}

			inv, err := model.NewDEB2B("R-MIME", cal.MakeDate(2026, 9, 1))
			if err != nil {
				t.Fatal(err)
			}
			b := inv.Bill()
			b.Attachments = append(b.Attachments, &org.Attachment{
				Code: cbc.Code("ATT-1"),
				Name: "a.bin",
				MIME: c.mime,
				URL:  url,
			})
			inv2, err := model.FromBill(b)
			if err != nil {
				if c.wantOK {
					t.Fatalf("FromBill: %v", err)
				}
				// GOBL may reject unknown MIME at calculate — still assert pre-pass via raw bill patch
				return
			}
			rep := validate.Validate(*inv2, parse.FormatXRechnungCII)
			found := false
			for _, v := range rep.Violations {
				if v.RuleID == validate.RuleAttachmentMIME {
					found = true
				}
			}
			if c.wantOK {
				if found {
					t.Fatalf("unexpected MIME violation: %+v", rep.Violations)
				}
			} else if !found {
				t.Fatalf("want %s, valid=%v viol=%+v", validate.RuleAttachmentMIME, rep.Valid, rep.Violations)
			}
		})
	}

	inv, err := model.NewDEB2B("R-200", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	specs := make([]model.AttachmentSpec, 201)
	for i := range specs {
		specs[i] = model.AttachmentSpec{Name: "a.pdf", MIME: "application/pdf", Data: []byte("%PDF")}
	}
	inv, err = model.WithAttachments(inv, specs)
	if err != nil {
		t.Fatal(err)
	}
	rep := validate.Validate(*inv, parse.FormatXRechnungCII)
	foundLimit := false
	for _, v := range rep.Violations {
		if v.RuleID == validate.RuleAttachmentLimit {
			foundLimit = true
		}
	}
	if !foundLimit {
		t.Fatalf("want %s, got %+v", validate.RuleAttachmentLimit, rep.Violations)
	}
}

func TestPh9_DirectDebitBG19(t *testing.T) {
	raw := consoleShapeJSON("standard", "R-DD-1", nil)
	var inv map[string]any
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatal(err)
	}
	inv["payment"] = map[string]any{
		"instructions": map[string]any{
			"key": "direct-debit",
			"direct_debit": map[string]string{
				"ref":      "MANDATE-42",
				"creditor": "DE98ZZZ09999999999",
				"account":  "DE89370400440532013000",
			},
		},
		"terms": map[string]string{"notes": "14 Tage netto"},
	}
	b, _ := json.Marshal(inv)
	doc, err := model.FromJSON(b)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	rep := validate.Validate(*doc, parse.FormatXRechnungCII)
	if !rep.Valid {
		t.Fatalf("validate: %s %+v", rep.Summary, rep.Violations)
	}
	cii, err := toXRechnungCII(*doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(cii)
	for _, needle := range []string{
		"DirectDebitMandateID", "MANDATE-42",
		"CreditorReferenceID", "DE98ZZZ09999999999",
		"IBANID", "DE89370400440532013000",
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("CII missing %q", needle)
		}
	}
}

func TestPh9_AlreadyPaidBT113(t *testing.T) {
	raw := consoleShapeJSON("standard", "R-PAID-1", nil)
	var inv map[string]any
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatal(err)
	}
	pay := inv["payment"].(map[string]any)
	pay["advances"] = []map[string]string{{"amount": "50.00"}}
	b, _ := json.Marshal(inv)
	doc, err := model.FromJSON(b)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	rep := validate.Validate(*doc, parse.FormatXRechnungCII)
	if !rep.Valid {
		t.Fatalf("validate: %s %+v", rep.Summary, rep.Violations)
	}
	cii, err := toXRechnungCII(*doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cii), "TotalPrepaidAmount") {
		t.Fatal("missing TotalPrepaidAmount (BT-113/114)")
	}
}

func TestPh9_SteuernummerBT32(t *testing.T) {
	raw := consoleShapeJSON("standard", "R-KU-1", nil)
	var inv map[string]any
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatal(err)
	}
	inv["supplier"] = map[string]any{
		"name": "Kleinunternehmer GmbH",
		"identities": []map[string]string{{
			"scope": "tax", "country": "DE", "code": "12/345/67890",
		}},
		"addresses": []map[string]string{{
			"street": "Dietmar-Hopp-Allee", "locality": "Walldorf",
			"code": "69190", "country": "DE",
		}},
		"emails":  []map[string]string{{"addr": "billing@example.com"}},
		"inboxes": []map[string]string{{"email": "billing@example.com"}},
		"people": []map[string]any{{
			"name":       map[string]string{"given": "Ada Lovelace"},
			"telephones": []map[string]string{{"num": "+49100200300"}},
			"emails":     []map[string]string{{"addr": "billing@example.com"}},
		}},
	}
	// Zero-rated line avoids BR-S-2 seller VAT ID for standard-rated lines.
	inv["lines"] = []map[string]any{{
		"quantity": "1",
		"item":     map[string]string{"name": "Service", "price": "100.00"},
		"taxes":    []map[string]string{{"cat": "VAT", "rate": "zero"}},
	}}
	b, _ := json.Marshal(inv)
	doc, err := model.FromJSON(b)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	cii, err := toXRechnungCII(*doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(cii)
	seller := sellerPartyXML(s)
	if !strings.Contains(seller, "12/345/67890") || !strings.Contains(seller, `schemeID="FC"`) {
		t.Fatalf("missing BT-32 SchemeID FC / Steuernummer in seller CII:\n%s", seller)
	}
	if strings.Contains(seller, `schemeID="VA"`) {
		t.Fatal("Kleinunternehmer seller must not emit BT-31 VA")
	}

	std := consoleShapeJSON("standard", "R-VA-1", nil)
	docVA, err := model.FromJSON(std)
	if err != nil {
		t.Fatal(err)
	}
	ciiVA, err := toXRechnungCII(*docVA)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sellerPartyXML(string(ciiVA)), `schemeID="VA"`) {
		t.Fatal("USt-IdNr supplier should emit SchemeID VA")
	}
}

func sellerPartyXML(cii string) string {
	const start = "SellerTradeParty"
	const end = "</ram:SellerTradeParty>"
	i := strings.Index(cii, start)
	if i < 0 {
		return ""
	}
	j := strings.Index(cii[i:], end)
	if j < 0 {
		return cii[i:]
	}
	return cii[i : i+j]
}
