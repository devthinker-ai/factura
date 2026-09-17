package validate

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/invopop/gobl/bill"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
)

// Stable Factura pre-pass RuleIDs (issuance gating go-xinvoice does not own).
const (
	RulePrecedingRequired = "FACTURA-PRECEDING-REQUIRED"
	RulePrecedingDate     = "FACTURA-PRECEDING-DATE"
	RuleAttachmentLimit   = "FACTURA-ATTACHMENT-LIMIT"
	RuleAttachmentEmpty   = "FACTURA-ATTACHMENT-EMPTY"
	RuleAttachmentMIME    = "FACTURA-ATTACHMENT-MIME"
	RuleLeitweg           = "FACTURA-LEITWEG"
)

// MaxAttachments is the EN 16931 / XRechnung practical cap (BT-125).
// go-xinvoice does not enforce a count — we gate at 200.
const MaxAttachments = 200

// allowedAttachmentMIMEs is the BT-125 allow-list (e-rechnung.bund.de /
// EN 16931 / XRechnung). image/jpg is accepted as an alias of image/jpeg.
var allowedAttachmentMIMEs = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {},
	"application/vnd.oasis.opendocument.spreadsheet":                    {},
	"text/csv": {},
}

// leitwegIDRe is the KoSIT Leitweg-ID syntax (BT-10 for B2G):
//   ^[0-9A-Z]{2,12}(-[0-9A-Z]{0,30})?-[0-9]{2}$
// Documented in AGENTS.md. go-xinvoice only checks presence (BR-DE-15).
var leitwegIDRe = regexp.MustCompile(`^[0-9A-Z]{2,12}(-[0-9A-Z]{0,30})?-[0-9]{2}$`)

// IsValidLeitwegID reports whether s matches the KoSIT Leitweg-ID syntax.
func IsValidLeitwegID(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	return s != "" && leitwegIDRe.MatchString(s)
}

// Validate runs issuance pre-pass checks, then go-xinvoice ValidateXML via CII.
func Validate(inv model.Invoice, src parse.Format) Report {
	format := string(src)
	if format == "" || format == string(parse.FormatUnknown) {
		format = string(parse.FormatXRechnungCII)
	}

	if pre := issuancePrePass(inv); len(pre) > 0 {
		rep := Report{
			Valid:      false,
			Format:     format,
			Level:      "business",
			Violations: pre,
		}
		if len(pre) == 1 {
			rep.Summary = "Invalid: " + pre[0].Human
		} else {
			rep.Summary = fmt.Sprintf("Invalid: %d rule violation(s)", len(pre))
		}
		return rep
	}
	return validateGOBLCII(inv, format)
}

func issuancePrePass(inv model.Invoice) []Violation {
	b := inv.Bill()
	if b == nil {
		return nil
	}
	var out []Violation

	needsPreceding := b.Type == bill.InvoiceTypeCreditNote || b.Type == bill.InvoiceTypeCorrective
	if needsPreceding {
		if len(b.Preceding) == 0 {
			out = append(out, Violation{
				RuleID: RulePrecedingRequired,
				Human:  humanFor(RulePrecedingRequired, "", ""),
			})
		} else {
			for _, p := range b.Preceding {
				if p == nil {
					continue
				}
				if p.Code != "" && (p.IssueDate == nil || !p.IssueDate.IsValid() || p.IssueDate.IsZero()) {
					out = append(out, Violation{
						RuleID: RulePrecedingDate,
						Human:  humanFor(RulePrecedingDate, "", ""),
						Value:  string(p.Code),
					})
					break
				}
			}
		}
	}

	if n := len(b.Attachments); n > MaxAttachments {
		out = append(out, Violation{
			RuleID: RuleAttachmentLimit,
			Human:  humanFor(RuleAttachmentLimit, "", ""),
			Value:  fmt.Sprintf("%d", n),
		})
	}
	for _, a := range b.Attachments {
		if a == nil {
			continue
		}
		if emptyDataURI(a.URL) {
			out = append(out, Violation{
				RuleID: RuleAttachmentEmpty,
				Human:  humanFor(RuleAttachmentEmpty, "", ""),
				Value:  a.Name,
			})
			break
		}
	}
	for _, a := range b.Attachments {
		if a == nil {
			continue
		}
		mime := effectiveAttachmentMIME(a.MIME, a.URL)
		if !isAllowedAttachmentMIME(mime) {
			out = append(out, Violation{
				RuleID: RuleAttachmentMIME,
				Human:  humanFor(RuleAttachmentMIME, "", ""),
				Value:  firstNonEmpty(a.Name, mime),
			})
			break
		}
	}

	if b.Ordering != nil {
		code := strings.TrimSpace(string(b.Ordering.Code))
		if looksLikeLeitwegAttempt(code) && !IsValidLeitwegID(code) {
			out = append(out, Violation{
				RuleID: RuleLeitweg,
				Human:  humanFor(RuleLeitweg, "", ""),
				Value:  code,
			})
		}
	}

	return out
}

// looksLikeLeitwegAttempt: values that contain a hyphen and start with a digit
// (typical Leitweg routing) — B2B buyer refs like "PO-42" / "NA" are exempt.
func looksLikeLeitwegAttempt(code string) bool {
	if code == "" || code == "NA" {
		return false
	}
	u := strings.ToUpper(code)
	if !strings.Contains(u, "-") {
		return false
	}
	// Digit-led routing IDs (04011000-…); letter-led PO refs stay exempt.
	r := u[0]
	return r >= '0' && r <= '9'
}

func emptyDataURI(url string) bool {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(strings.ToLower(url), "data:") {
		return false // external URL — not our empty-payload check
	}
	// data:[<mime>][;base64],<payload>
	comma := strings.Index(url, ",")
	if comma < 0 {
		return true
	}
	payload := strings.TrimSpace(url[comma+1:])
	if payload == "" {
		return true
	}
	header := strings.ToLower(url[:comma])
	if strings.Contains(header, ";base64") {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			// Try RawStdEncoding (no padding).
			decoded, err = base64.RawStdEncoding.DecodeString(payload)
		}
		return err != nil || len(decoded) == 0
	}
	return false
}

// effectiveAttachmentMIME prefers the explicit MIME field; otherwise parses
// the data:<mime>;… prefix from a data-URI URL.
func effectiveAttachmentMIME(mime, url string) string {
	if m := normalizeAttachmentMIME(mime); m != "" {
		return m
	}
	return mimeFromDataURI(url)
}

func mimeFromDataURI(url string) string {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(strings.ToLower(url), "data:") {
		return ""
	}
	rest := url[len("data:"):]
	semi := strings.IndexAny(rest, ";,")
	if semi < 0 {
		return normalizeAttachmentMIME(rest)
	}
	return normalizeAttachmentMIME(rest[:semi])
}

func normalizeAttachmentMIME(mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if m == "image/jpg" {
		return "image/jpeg"
	}
	return m
}

func isAllowedAttachmentMIME(mime string) bool {
	_, ok := allowedAttachmentMIMEs[normalizeAttachmentMIME(mime)]
	return ok
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
