// Package validate runs EN 16931 + XRechnung business rules and maps findings
// to human-readable reports.
//
// Spike pin: xinvoice.ValidateXML(bytes) → ValidationResult with Finding{Rule,
// Severity, Location, Detail}; bilingual text via Result.JSON("de"|"en").
// See Spike.md §d. Offline / deterministic — no network.
package validate

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	xinvoice "github.com/andeedotnet/go-xinvoice"
	cii "github.com/invopop/gobl.cii"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
)

// Report is the product validation output.
type Report struct {
	Valid      bool        `json:"valid"`
	Format     string      `json:"format"`
	Level      string      `json:"level"` // "schema" | "business"
	Violations []Violation `json:"violations"`
	Summary    string      `json:"summary"`
}

// Violation is one rule failure with human text.
type Violation struct {
	RuleID string `json:"rule_id"`
	Human  string `json:"human"`
	Value  string `json:"value,omitempty"`
}

// catalogFinding is the shape of go-xinvoice Result.JSON entries.
type catalogFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Location string `json:"location,omitempty"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

type catalogResult struct {
	Valid    bool             `json:"valid"`
	Findings []catalogFinding `json:"findings"`
}

// ValidateBytes validates original invoice bytes (preferred receive path).
// WHY raw bytes: round-tripping through GOBL can recalculate totals and hide
// amount mismatches present in the source document.
func ValidateBytes(data []byte) Report {
	format := parse.DetectFormat(data)
	xmlBytes, level, err := extractXML(data, format)
	if err != nil {
		return Report{
			Valid:  false,
			Format: string(format),
			Level:  "schema",
			Violations: []Violation{{
				RuleID: "XML-PARSE",
				Human:  humanFor("XML-PARSE", "", ""),
				Value:  err.Error(),
			}},
			Summary: "Parse/schema error — cannot validate business rules",
		}
	}
	return fromXInvoice(xinvoice.ValidateXML(xmlBytes), string(format), level)
}

// validateGOBLCII converts a GOBL invoice to XRechnung CII and validates it.
// Spike pin: cii.ConvertInvoice(..., ContextXRechnungV3) + Bytes → ValidateXML.
// Public entry is Validate (prepass.go), which runs issuance gating first.
func validateGOBLCII(inv model.Invoice, format string) Report {
	env := inv.Envelope()
	if env == nil {
		return Report{
			Valid:   false,
			Format:  format,
			Level:   "schema",
			Summary: "No GOBL envelope to validate",
			Violations: []Violation{{
				RuleID: "XML-PARSE",
				Human:  "GOBL-Dokument fehlt / GOBL document missing",
			}},
		}
	}
	doc, err := cii.ConvertInvoice(env, cii.WithContext(cii.ContextXRechnungV3))
	if err != nil {
		return Report{
			Valid:  false,
			Format: format,
			Level:  "schema",
			Violations: []Violation{{
				RuleID: "XML-PARSE",
				Human:  "CII-Konvertierung fehlgeschlagen / CII conversion failed",
				Value:  err.Error(),
			}},
			Summary: "Could not convert GOBL to XRechnung CII for validation",
		}
	}
	xmlBytes, err := doc.Bytes()
	if err != nil {
		return Report{
			Valid:  false,
			Format: format,
			Level:  "schema",
			Violations: []Violation{{
				RuleID: "XML-PARSE",
				Human:  "CII-Serialisierung fehlgeschlagen / CII serialization failed",
				Value:  err.Error(),
			}},
			Summary: "Could not serialize XRechnung CII for validation",
		}
	}
	return fromXInvoice(xinvoice.ValidateXML(xmlBytes), format, "business")
}

func extractXML(data []byte, format parse.Format) ([]byte, string, error) {
	switch format {
	case parse.FormatXRechnungCII, parse.FormatXRechnungUBL:
		return data, "business", nil
	case parse.FormatZUGFeRDPDF, parse.FormatFacturXPDF:
		xml, _, err := extractPDF(data)
		if err != nil {
			return nil, "schema", err
		}
		return xml, "business", nil
	default:
		// Try ValidateXML anyway — it auto-detects UBL/CII.
		trim := bytesTrimSpace(data)
		if len(trim) > 0 && (trim[0] == '<' || hasXMLDecl(trim)) {
			return data, "business", nil
		}
		return nil, "schema", fmt.Errorf("not an XML or hybrid-PDF invoice")
	}
}

func extractPDF(data []byte) ([]byte, string, error) {
	// Imported via parse path; call pdf extract through ValidateBytes using
	// the same library. Avoid import cycle by duplicating the call here.
	return pdfExtract(data)
}

// pdfExtract is assigned in pdf_extract.go to call xinvoicepdf without cycles.
var pdfExtract = func(data []byte) ([]byte, string, error) {
	return nil, "", fmt.Errorf("pdf extract not wired")
}

func fromXInvoice(res *xinvoice.ValidationResult, format, level string) Report {
	if res == nil {
		return Report{Valid: false, Format: format, Level: level, Summary: "Empty validation result"}
	}

	deJSON, _ := res.JSON("de")
	enJSON, _ := res.JSON("en")
	var deCat, enCat catalogResult
	_ = json.Unmarshal(deJSON, &deCat)
	_ = json.Unmarshal(enJSON, &enCat)
	enByRule := map[string]string{}
	for _, f := range enCat.Findings {
		enByRule[f.Rule] = f.Message
	}

	rep := Report{
		Valid:  res.Valid(),
		Format: format,
		Level:  level,
	}
	for _, f := range deCat.Findings {
		if f.Severity != "" && f.Severity != "error" {
			// Product report focuses on errors; warnings omitted from Violations
			// but do not affect Valid() (library already ignores them for Valid).
			continue
		}
		v := Violation{
			RuleID: f.Rule,
			Human:  humanFor(f.Rule, f.Message, enByRule[f.Rule]),
			Value:  redactValue(f.Detail),
		}
		rep.Violations = append(rep.Violations, v)
	}
	// Also include typed Findings that JSON might have skipped (defensive).
	if len(rep.Violations) == 0 && !res.Valid() {
		for _, f := range res.Findings {
			if f.Severity != xinvoice.SeverityError {
				continue
			}
			rep.Violations = append(rep.Violations, Violation{
				RuleID: f.Rule,
				Human:  humanFor(f.Rule, "", ""),
				Value:  redactValue(f.Detail),
			})
		}
	}

	switch {
	case rep.Valid:
		rep.Summary = "Invoice is valid against EN 16931 / XRechnung business rules"
	case len(rep.Violations) == 1:
		rep.Summary = "Invalid: " + rep.Violations[0].Human
	default:
		rep.Summary = fmt.Sprintf("Invalid: %d rule violation(s)", len(rep.Violations))
	}
	return rep
}

// redactValue masks tax IDs / long identifiers to last 4 runes.
func redactValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// If detail looks like it contains a DE VAT ID, redact it.
	if i := strings.Index(s, "DE"); i >= 0 && i+2 < len(s) {
		rest := s[i+2:]
		digits := 0
		for _, r := range rest {
			if r >= '0' && r <= '9' {
				digits++
			} else {
				break
			}
		}
		if digits >= 9 {
			return s[:i+2] + "*****" + rest[digits-4:digits] + rest[digits:]
		}
	}
	if utf8.RuneCountInString(s) > 24 {
		runes := []rune(s)
		return "…" + string(runes[len(runes)-4:])
	}
	return s
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func hasXMLDecl(data []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(data)), "<?xml")
}
