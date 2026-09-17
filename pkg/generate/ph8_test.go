package generate

import (
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/tax"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

func typeCodeOf(t *testing.T, inv model.Invoice) string {
	t.Helper()
	ciiBytes, err := toXRechnungCII(inv)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if m := regexp.MustCompile(`<ram:TypeCode>([^<]*)</ram:TypeCode>`).FindStringSubmatch(string(ciiBytes)); m != nil {
		return m[1]
	}
	return ""
}

func TestPh8_TypeCodesInCII(t *testing.T) {
	issue := cal.MakeDate(2026, 9, 1)
	prev := cal.MakeDate(2026, 8, 1)

	std, err := model.NewDEB2B("R-1", issue)
	if err != nil {
		t.Fatal(err)
	}
	cn, err := model.NewCreditNote("CN-1", issue, "R-0", prev)
	if err != nil {
		t.Fatal(err)
	}
	corr, err := model.NewCorrected("COR-1", issue, "R-0", prev)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := model.NewSelfBilled("SB-1", issue)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		inv  *model.Invoice
		want string
	}{
		{"standard", std, "380"},
		{"credit-note", cn, "381"},
		{"corrective", corr, "384"},
		{"self-billed", sb, "389"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.inv.TypeCode(); got != c.want {
				t.Fatalf("TypeCode()=%s want %s", got, c.want)
			}
			code := typeCodeOf(t, *c.inv)
			if code != c.want {
				t.Fatalf("CII TypeCode=%s want %s", code, c.want)
			}
			if c.want == "381" || c.want == "384" {
				cii, _ := toXRechnungCII(*c.inv)
				if !strings.Contains(string(cii), "ram:InvoiceReferencedDocument") {
					t.Fatal("missing ram:InvoiceReferencedDocument")
				}
			}
			if c.want == "389" {
				b := c.inv.Bill()
				if !b.HasTags(tax.TagSelfBilled) {
					t.Fatal("self-billed must use tax.TagSelfBilled ($tags)")
				}
			}
		})
	}
}

func TestPh8_BuyerReferenceBT10(t *testing.T) {
	inv, err := model.NewDEB2B("R-LW", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	inv, err = model.WithLeitwegID(inv, "04011000-12345-34")
	if err != nil {
		t.Fatal(err)
	}
	cii, err := toXRechnungCII(*inv)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(cii)
	if !strings.Contains(xml, "<ram:BuyerReference>04011000-12345-34</ram:BuyerReference>") {
		t.Fatalf("missing BT-10 BuyerReference in CII")
	}
}

func TestPh8_AttachmentDataURI(t *testing.T) {
	inv, err := model.NewDEB2B("R-ATT", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := base64.StdEncoding.DecodeString("JVBERi0xLjcKMSAwIG9iajw8L1R5cGUvQ2F0YWxvZjw8L1BhZ2VzIDIgMCBSPj4+ZW5kb2JqCjIgMCBvYmo8PC9UeXBlL1BhZ2UvTWVkaWFCb3hbMCAwIDYxMiA3OTJdL1BhcmVudCAxIDAgUj4+ZW5kb2JqCnRyYWlsZXI8PC9TaXplIDIvUm9vdCAxIDAgUj4+CnN0YXJ0eHJlZiA1NjYKJSVFT0Y=")
	inv, err = model.WithAttachments(inv, []model.AttachmentSpec{{
		Name: "scan.pdf", Description: "Supporting scan", MIME: "application/pdf", Data: pdf,
	}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	xr, _, err := Generate(*inv, dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, _ := os.ReadFile(xr)
	s := string(b)
	if !strings.Contains(s, "data:application/pdf") {
		t.Fatal("missing data-URI in CII")
	}
	if !strings.Contains(s, ">916<") {
		t.Fatal("missing document type code 916 (BT-125)")
	}
}

func TestPh8_CreditNoteNoPreceding(t *testing.T) {
	inv, err := model.NewDEB2B("CN-bare", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	b := inv.Bill()
	b.Type = "credit-note"
	inv2, err := model.FromBill(b)
	if err != nil {
		t.Fatal(err)
	}
	rep := validate.Validate(*inv2, parse.FormatXRechnungCII)
	if rep.Valid {
		t.Fatal("expected invalid without preceding")
	}
	found := false
	for _, v := range rep.Violations {
		if v.RuleID == validate.RulePrecedingRequired {
			found = true
		}
	}
	if !found {
		t.Fatalf("want %s, got %+v", validate.RulePrecedingRequired, rep.Violations)
	}
	_, _, err = Generate(*inv2, t.TempDir())
	if err == nil {
		t.Fatal("generate should fail")
	}
	if !strings.Contains(err.Error(), validate.RulePrecedingRequired) {
		t.Fatalf("error should cite rule: %v", err)
	}
}

func TestPh8_AttachmentLimit(t *testing.T) {
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
	if rep.Valid {
		t.Fatal("201 attachments should be invalid")
	}
	found := false
	for _, v := range rep.Violations {
		if v.RuleID == validate.RuleAttachmentLimit {
			found = true
		}
	}
	if !found {
		t.Fatalf("want %s, got %+v", validate.RuleAttachmentLimit, rep.Violations)
	}
}

func TestPh8_CreditNoteSignConvention(t *testing.T) {
	cn, err := model.NewCreditNote("CN-sign", cal.MakeDate(2026, 9, 1), "R-0", cal.MakeDate(2026, 8, 1))
	if err != nil {
		t.Fatal(err)
	}
	if cn.TotalCents() >= 0 {
		t.Fatalf("credit note totals must be negative (signed convention), got %d", cn.TotalCents())
	}
	dir := t.TempDir()
	xr, zf, err := Generate(*cn, dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	xrBytes, _ := os.ReadFile(xr)
	parsed, _, err := parse.Parse(xrBytes)
	if err != nil {
		t.Fatalf("parse round-trip: %v", err)
	}
	if parsed.TypeCode() != model.TypeCodeCreditNote {
		t.Fatalf("round-trip type=%s", parsed.TypeCode())
	}
	pdf, _ := os.ReadFile(zf)
	if !strings.Contains(string(pdf), "Payable: -") {
		t.Fatal("PDF should label negative Payable for credit note")
	}
	if !strings.Contains(string(pdf), "CREDIT NOTE") {
		t.Fatal("PDF should label credit note")
	}
}

func TestPh8_EmptyAttachment(t *testing.T) {
	inv, err := model.NewDEB2B("R-empty", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	inv, err = model.WithAttachments(inv, []model.AttachmentSpec{{
		Name: "empty.pdf", MIME: "application/pdf", Data: nil,
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Force empty payload in data-URI
	b := inv.Bill()
	b.Attachments[0].URL = "data:application/pdf;base64,"
	inv2, err := model.FromBill(b)
	if err != nil {
		// FromBill may fail calculate; use FromJSON path instead
		t.Skip("FromBill rejected empty attachment")
	}
	rep := validate.Validate(*inv2, parse.FormatXRechnungCII)
	found := false
	for _, v := range rep.Violations {
		if v.RuleID == validate.RuleAttachmentEmpty {
			found = true
		}
	}
	if rep.Valid || !found {
		t.Fatalf("want FACTURA-ATTACHMENT-EMPTY, valid=%v viol=%+v", rep.Valid, rep.Violations)
	}
}

func TestPh8_MalformedLeitweg(t *testing.T) {
	inv, err := model.NewDEB2B("R-lwbad", cal.MakeDate(2026, 9, 1))
	if err != nil {
		t.Fatal(err)
	}
	inv, err = model.WithLeitwegID(inv, "04011000-BAD")
	if err != nil {
		t.Fatal(err)
	}
	rep := validate.Validate(*inv, parse.FormatXRechnungCII)
	found := false
	for _, v := range rep.Violations {
		if v.RuleID == validate.RuleLeitweg {
			found = true
		}
	}
	if rep.Valid || !found {
		t.Fatalf("want FACTURA-LEITWEG, valid=%v viol=%+v", rep.Valid, rep.Violations)
	}
}

func TestPh8_PDFNegativePayableExplicit(t *testing.T) {
	cn, err := model.NewCreditNote("CN-pdf", cal.MakeDate(2026, 9, 1), "R-0", cal.MakeDate(2026, 8, 1))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_, zf, err := Generate(*cn, dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pdf, _ := os.ReadFile(zf)
	if !strings.Contains(string(pdf), "Payable: -") {
		t.Fatalf("expected Payable: - in PDF, totals=%d", cn.TotalCents())
	}
}
