// Package parse detects e-invoice formats and converts them to GOBL.
//
// Spike pin: CII → gobl.cii.Parse; UBL → gobl.ubl.Parse + Convert;
// PDF hybrid → xinvoicepdf.ExtractXML then cii.Parse. See Spike.md §c.
package parse

import (
	"bytes"
	"fmt"
	"strings"

	xinvoicepdf "github.com/andeedotnet/go-xinvoice-pdf"
	cii "github.com/invopop/gobl.cii"
	ubl "github.com/invopop/gobl.ubl"

	"github.com/devthinker-ai/factura/pkg/model"
)

// Format is a detected e-invoice syntax.
type Format string

// Supported formats. WHY these five: DE receive mandate accepts XRechnung
// (CII or UBL) and ZUGFeRD/Factur-X hybrid PDFs; everything else is unknown.
const (
	FormatXRechnungCII Format = "xrechnung_cii"
	FormatXRechnungUBL Format = "xrechnung_ubl"
	FormatZUGFeRDPDF   Format = "zugferd_pdf"
	FormatFacturXPDF   Format = "facturx_pdf"
	FormatUnknown      Format = "unknown"
)

// Error is a typed parse failure (archive status=parse_error; CLI exit 2).
type Error struct {
	Reason string
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("parse: %s: %v", e.Reason, e.Cause)
	}
	return "parse: " + e.Reason
}

func (e *Error) Unwrap() error { return e.Cause }

// DetectFormat sniffs bytes without fully parsing.
//
// WHY order: PDF magic first (binary), then XML namespaces for CII vs UBL,
// then guideline/profile hints to distinguish XRechnung vs ZUGFeRD/Factur-X.
func DetectFormat(data []byte) Format {
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 {
		return FormatUnknown
	}
	if bytes.HasPrefix(trim, []byte("%PDF")) {
		xml, name, err := xinvoicepdf.ExtractXML(trim)
		if err != nil {
			return FormatUnknown
		}
		lower := strings.ToLower(name)
		// WHY: embedded part name is the reliable Factur-X vs ZUGFeRD signal.
		switch {
		case strings.Contains(lower, "factur"):
			return FormatFacturXPDF
		case strings.Contains(lower, "zugferd") || strings.Contains(lower, "zf"):
			return FormatZUGFeRDPDF
		default:
			// Profile fallback from XML guideline ID.
			s := string(xml)
			if strings.Contains(s, "factur-x") || strings.Contains(s, "FACTUR-X") {
				return FormatFacturXPDF
			}
			return FormatZUGFeRDPDF
		}
	}
	if !looksLikeXML(trim) {
		return FormatUnknown
	}
	s := string(trim)
	switch {
	case strings.Contains(s, "CrossIndustryInvoice") ||
		strings.Contains(s, "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice"):
		// XRechnung CII uses kosit guideline; ZUGFeRD-as-XML is rare on the wire
		// as bare XML (usually PDF). Treat CII XML as xrechnung_cii.
		return FormatXRechnungCII
	case strings.Contains(s, "urn:oasis:names:specification:ubl:schema:xsd:Invoice") ||
		(strings.Contains(s, "Invoice") && strings.Contains(s, "urn:oasis:names:specification:ubl")):
		return FormatXRechnungUBL
	default:
		return FormatUnknown
	}
}

// Parse detects format and returns a GOBL invoice view.
func Parse(data []byte) (model.Invoice, Format, error) {
	fmtDetect := DetectFormat(data)
	if fmtDetect == FormatUnknown {
		return model.Invoice{}, FormatUnknown, &Error{Reason: "unrecognized invoice format"}
	}

	switch fmtDetect {
	case FormatXRechnungCII:
		inv, err := parseCII(data)
		if err != nil {
			return model.Invoice{}, fmtDetect, err
		}
		return *inv, fmtDetect, nil
	case FormatXRechnungUBL:
		inv, err := parseUBL(data)
		if err != nil {
			return model.Invoice{}, fmtDetect, err
		}
		return *inv, fmtDetect, nil
	case FormatZUGFeRDPDF, FormatFacturXPDF:
		xml, _, err := xinvoicepdf.ExtractXML(data)
		if err != nil {
			return model.Invoice{}, fmtDetect, &Error{Reason: "pdf extract failed", Cause: err}
		}
		inv, err := parseCII(xml)
		if err != nil {
			return model.Invoice{}, fmtDetect, err
		}
		return *inv, fmtDetect, nil
	default:
		return model.Invoice{}, FormatUnknown, &Error{Reason: "unrecognized invoice format"}
	}
}

func parseCII(data []byte) (*model.Invoice, error) {
	// Spike pin: cii.Parse(data) (*gobl.Envelope, error) — gobl.cii/cii.go
	env, err := cii.Parse(data)
	if err != nil {
		return nil, &Error{Reason: "cii parse failed", Cause: err}
	}
	inv, err := model.FromEnvelope(env)
	if err != nil {
		return nil, &Error{Reason: "gobl wrap failed", Cause: err}
	}
	return inv, nil
}

func parseUBL(data []byte) (*model.Invoice, error) {
	// Spike pin: ubl.Parse → (*ubl.Invoice).Convert — gobl.ubl
	doc, err := ubl.Parse(data)
	if err != nil {
		return nil, &Error{Reason: "ubl parse failed", Cause: err}
	}
	ui, ok := doc.(*ubl.Invoice)
	if !ok {
		return nil, &Error{Reason: fmt.Sprintf("ubl: unexpected type %T", doc)}
	}
	env, err := ui.Convert()
	if err != nil {
		return nil, &Error{Reason: "ubl convert failed", Cause: err}
	}
	inv, err := model.FromEnvelope(env)
	if err != nil {
		return nil, &Error{Reason: "gobl wrap failed", Cause: err}
	}
	return inv, nil
}

func looksLikeXML(data []byte) bool {
	if bytes.HasPrefix(data, []byte("<?xml")) {
		return true
	}
	if len(data) > 0 && data[0] == '<' {
		return true
	}
	// BOM + xml
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return looksLikeXML(data[3:])
	}
	return false
}
