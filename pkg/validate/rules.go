package validate

// Human messages for DE-critical EN 16931 / XRechnung rules.
//
// WHY we own this table: go-xinvoice already ships official Schematron text via
// Result.JSON("de"|"en"), but product UX wants short bilingual one-liners.
// Unmapped rules still surface — never swallowed (see humanFor).

var ruleHuman = map[string]string{
	// Identification
	"BR-1":  "Rechnungsnummer fehlt / invoice number (BT-1) missing",
	"BR-2":  "Rechnungsnummer fehlt / invoice number (BT-1) missing",
	"BR-3":  "Rechnungsdatum fehlt / invoice issue date (BT-2) missing",
	"BR-4":  "Rechnungstypcode fehlt / invoice type code (BT-3) missing",
	"BR-5":  "Währungscode fehlt / invoice currency code (BT-5) missing",
	"BR-6":  "Verkäuferangaben fehlen / seller (BG-4) missing",
	"BR-7":  "Verkäufername fehlt / seller name (BT-27) missing",
	"BR-8":  "Verkäuferanschrift fehlt / seller postal address (BG-5) missing",
	"BR-9":  "Verkäuferland fehlt / seller country code (BT-40) missing",
	"BR-10": "Verkäufersteuerinfo fehlt / seller tax registration missing",
	"BR-11": "Käuferangaben fehlen / buyer (BG-7) missing",
	"BR-12": "Käufername fehlt / buyer name (BT-44) missing",

	// VAT / tax IDs
	"BR-CO-09": "Verkäufer-USt-IdNr fehlt oder ungültig / seller VAT ID missing or invalid",
	"BR-CO-26": "Verkäuferkennungen fehlen (BT-29/30/31) / seller identifiers missing",
	"BR-S-2":   "Verkäufer-USt-IdNr fehlt bei Steuersatz S / seller VAT ID required for standard-rated lines",
	"BR-DE-16": "USt-IdNr/Steuernummer des Verkäufers fehlt / seller VAT ID or tax number missing (BT-31/32)",
	"BR-AE-2":  "USt-IdNr des Käufers fehlt bei reverse-charge / buyer VAT ID required for reverse charge",
	"BR-IC-2":  "USt-IdNr des Käufers fehlt bei innergemeinschaftlicher Lieferung / buyer VAT ID required for intra-community supply",

	// Amounts
	"BR-CO-10": "Summe der Positionen stimmt nicht / sum of line net amounts mismatch (BT-106)",
	"BR-CO-13": "Summe der Steuerbeträge stimmt nicht / invoice total VAT amount mismatch (BT-110)",
	"BR-CO-14": "Nettobetrag stimmt nicht / invoice total without VAT mismatch (BT-109)",
	"BR-CO-15": "Bruttobetrag stimmt nicht mit Netto+USt überein / total with VAT ≠ net + VAT (BT-112)",
	"BR-CO-16": "Fälliger Betrag stimmt nicht / amount due for payment mismatch (BT-115)",
	"BR-CO-17": "Steuerbasisbetrag stimmt nicht / VAT category taxable amount mismatch",

	// DE XRechnung specifics
	"BR-DE-1":  "Zahlungsangaben (BG-16) fehlen / payment instructions missing",
	"BR-DE-2":  "Verkäuferkontakt (BG-6) fehlt / seller contact missing",
	"BR-DE-15": "Käuferreferenz (BT-10) fehlt / buyer reference missing",
	"BR-DE-18": "Skonto-Format in BT-20 ungültig / payment terms skonto format invalid",
	"BR-DE-21": "BT-24 entspricht nicht der XRechnung-Kennung / specification identifier not XRechnung",
	"BR-DE-23": "Überweisung erfordert BG-17 / credit transfer requires payment account (BG-17)",

	// Dates / syntax
	"BR-CO-03":  "Rechnungsdatum ungültig / invoice date invalid",
	"XML-PARSE": "XML konnte nicht gelesen werden / XML parse failed",

	// Phase 8 issuance pre-pass (go-xinvoice gaps)
	"FACTURA-PRECEDING-REQUIRED": "Vorhergehende Rechnung (BG-3) fehlt / preceding invoice reference (BG-3) required",
	"FACTURA-PRECEDING-DATE":     "Vorhergehendes Rechnungsdatum fehlt / preceding issue_date required when code is set",
	"FACTURA-ATTACHMENT-LIMIT":   "Mehr als 200 Anhänge (BT-125) / more than 200 attachments (BT-125)",
	"FACTURA-ATTACHMENT-EMPTY":   "Leerer data-URI-Anhang (BT-125) / empty or non-base64 data-URI attachment (BT-125)",
	"FACTURA-ATTACHMENT-MIME":    "Anhang-MIME-Typ nicht erlaubt (BT-125) / attachment MIME type not in BT-125 allow-list",
	"FACTURA-LEITWEG":            "Leitweg-ID (BT-10) ungültig / Leitweg-ID (BT-10) format invalid",
}

// humanFor returns our short bilingual line, or a non-swallowing fallback.
func humanFor(ruleID, libraryDE, libraryEN string) string {
	if h, ok := ruleHuman[ruleID]; ok {
		return h
	}
	// Prefer library catalog text when we have no mapping.
	switch {
	case libraryDE != "" && libraryEN != "":
		return libraryDE + " / " + libraryEN
	case libraryDE != "":
		return libraryDE
	case libraryEN != "":
		return libraryEN
	default:
		return "Rule " + ruleID + " violated (unmapped — see library docs)"
	}
}
