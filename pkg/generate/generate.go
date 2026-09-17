// Package generate produces XRechnung 3.0.2 CII and ZUGFeRD hybrid PDFs from GOBL.
//
// Spike pin (Spike.md §b):
//   XRechnung CII: cii.ConvertInvoice(env, WithContext(ContextXRechnungV3)).Bytes()
//   ZUGFeRD CII:   cii.ConvertInvoice(env, WithContext(ContextZUGFeRDV2)).Bytes()
//   PDF embed:     xinvoicepdf.Embed(basePDF, ciiXML, &Options{Profile: ProfileEN16931})
//
// WHY CII for issue: BZSt/KoSIT push UN/CEFACT CII as the primary DE syntax.
// WHY gobl.cii (not go-xinvoice) for GOBL→XML: GOBL-native path; never import
// both libraries for the same transform direction in one code path.
package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	xinvoicepdf "github.com/andeedotnet/go-xinvoice-pdf"
	cii "github.com/invopop/gobl.cii"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// Result paths from Generate.
type Result struct {
	XRechnungPath string
	ZUGFeRDPath   string
}

// Generate writes <invoice-number>-xrechnung.xml and <invoice-number>-zugferd.pdf
// into outdir, then self-checks both artifacts with validate.ValidateBytes.
// If self-check fails, returns an error (we never ship documents we can't prove).
func Generate(inv model.Invoice, outdir string) (xrechnungPath, zugferdPath string, err error) {
	return GenerateWith(inv, outdir, nil)
}

// GenerateWith is Generate with optional PDF branding. tmpl nil = unbranded default.
// Branding never touches the XRechnung CII XML.
func GenerateWith(inv model.Invoice, outdir string, tmpl *Template) (xrechnungPath, zugferdPath string, err error) {
	if inv.Envelope() == nil {
		return "", "", fmt.Errorf("generate: nil invoice")
	}
	if outdir == "" {
		outdir = "."
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return "", "", fmt.Errorf("generate: mkdir: %w", err)
	}

	var t Template
	if tmpl != nil {
		t = *tmpl
	}

	// Pre-flight: GOBL → CII must validate before we write files.
	pre := validate.Validate(inv, parse.FormatXRechnungCII)
	if !pre.Valid {
		detail := pre.Summary
		if len(pre.Violations) > 0 {
			detail = pre.Violations[0].Human
			if pre.Violations[0].RuleID != "" {
				detail = pre.Violations[0].RuleID + ": " + pre.Violations[0].Human
			}
		}
		return "", "", fmt.Errorf("generate: invoice fails validation before emit: %s", detail)
	}

	base := sanitizeFilename(inv.InvoiceNumber())
	if base == "" {
		base = "invoice"
	}

	xrXML, err := toXRechnungCII(inv)
	if err != nil {
		return "", "", err
	}
	xrPath := filepath.Join(outdir, base+"-xrechnung.xml")
	if err := os.WriteFile(xrPath, xrXML, 0o644); err != nil {
		return "", "", fmt.Errorf("generate: write xrechnung: %w", err)
	}

	page, err := renderInvoicePDF(inv, t)
	if err != nil {
		_ = os.Remove(xrPath)
		return "", "", fmt.Errorf("generate: pdf page: %w", err)
	}
	// WHY embed XRechnung CII (not ContextZUGFeRDV2): ZUGFeRD EN16931 context
	// omits BT-23 (business process); go-xinvoice's KoSIT/XR rule set requires it.
	// DE accept XR-in-PDF hybrids; same CII as the .xml sibling keeps self-check
	// deterministic. ProfileXRechnung sets the XMP conformance level.
	hybrid, _, err := xinvoicepdf.Embed(page, xrXML, &xinvoicepdf.Options{
		Profile: xinvoicepdf.ProfileXRechnung,
	})
	if err != nil {
		_ = os.Remove(xrPath)
		return "", "", fmt.Errorf("generate: embed zugferd: %w", err)
	}
	zfPath := filepath.Join(outdir, base+"-zugferd.pdf")
	if err := os.WriteFile(zfPath, hybrid, 0o644); err != nil {
		_ = os.Remove(xrPath)
		return "", "", fmt.Errorf("generate: write zugferd: %w", err)
	}

	// Post-generate self-check on our own output.
	if rep := validate.ValidateBytes(xrXML); !rep.Valid {
		_ = os.Remove(xrPath)
		_ = os.Remove(zfPath)
		return "", "", fmt.Errorf("generate: self-check xrechnung failed: %s", rep.Summary)
	}
	if rep := validate.ValidateBytes(hybrid); !rep.Valid {
		_ = os.Remove(xrPath)
		_ = os.Remove(zfPath)
		return "", "", fmt.Errorf("generate: self-check zugferd failed: %s", rep.Summary)
	}
	return xrPath, zfPath, nil
}

func toXRechnungCII(inv model.Invoice) ([]byte, error) {
	doc, err := cii.ConvertInvoice(inv.Envelope(), cii.WithContext(cii.ContextXRechnungV3))
	if err != nil {
		return nil, fmt.Errorf("generate: cii xrechnung convert: %w", err)
	}
	b, err := doc.Bytes()
	if err != nil {
		return nil, fmt.Errorf("generate: cii xrechnung bytes: %w", err)
	}
	return b, nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer("/", "-", "\\", "-", " ", "_", ":", "-")
	s = repl.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
