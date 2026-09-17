package generate

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/invopop/gobl/org"
)

const (
	pageW       = 595.0
	pageH       = 842.0
	marginL     = 40.0
	marginR     = 40.0
	rowH        = 16.0
	maxRowsPage = 34
	logoH       = 55.0
)

type lineRow struct {
	pos   int
	name  string
	qty   string
	price string
	sum   string
}

// renderInvoicePDF builds a multi-page PDF 1.4 (Helvetica + optional JPEG logo).
// Branding is presentation-only; Payable: label is preserved for credit-note tests.
func renderInvoicePDF(inv model.Invoice, tmpl Template) ([]byte, error) {
	title := "INVOICE / RECHNUNG"
	switch inv.TypeCode() {
	case model.TypeCodeCreditNote:
		title = "CREDIT NOTE / GUTSCHRIFT"
	case model.TypeCodeCorrective:
		title = "CORRECTED INVOICE / KORREKTURRECHNUNG"
	case model.TypeCodeSelfBilled:
		title = "SELF-BILLED INVOICE / SELBSTABGERECHNETE RECHNUNG"
	}

	rows := collectLineRows(inv)
	pages := paginateRows(rows)

	var logoW, logoHpt float64
	var logoObjNum int
	if tmpl.HasLogo && len(tmpl.Logo) > 0 {
		iw, ih, err := LogoConfig(tmpl.Logo)
		if err != nil {
			return nil, fmt.Errorf("generate: logo config: %w", err)
		}
		if ih < 1 {
			ih = 1
		}
		logoHpt = logoH
		logoW = logoHpt * float64(iw) / float64(ih)
		if logoW > 180 {
			logoW = 180
			logoHpt = logoW * float64(ih) / float64(iw)
		}
	}

	type pageContent struct {
		stream []byte
	}
	built := make([]pageContent, len(pages))
	for pi, pageRows := range pages {
		last := pi == len(pages)-1
		built[pi].stream = buildPageStream(inv, tmpl, title, pageRows, pi+1, len(pages), last, logoW, logoHpt)
	}

	// Object layout:
	// 1 Catalog, 2 Pages, 3 F1, 4 F2, [5 Image], then pairs of (Page, Contents) per page.
	var out bytes.Buffer
	write := func(s string) { _, _ = out.WriteString(s) }
	offsets := map[int]int{}
	nextID := 1
	alloc := func() int {
		id := nextID
		nextID++
		return id
	}

	catalogID := alloc() // 1
	pagesID := alloc()   // 2
	f1ID := alloc()      // 3
	f2ID := alloc()      // 4
	if tmpl.HasLogo && len(tmpl.Logo) > 0 {
		logoObjNum = alloc()
	}

	pageIDs := make([]int, len(built))
	contentIDs := make([]int, len(built))
	for i := range built {
		pageIDs[i] = alloc()
		contentIDs[i] = alloc()
	}

	write("%PDF-1.4\n")

	offsets[catalogID] = out.Len()
	write(fmt.Sprintf("%d 0 obj<< /Type /Catalog /Pages %d 0 R >>endobj\n", catalogID, pagesID))

	offsets[pagesID] = out.Len()
	var kids strings.Builder
	kids.WriteString("[")
	for i, id := range pageIDs {
		if i > 0 {
			kids.WriteByte(' ')
		}
		kids.WriteString(fmt.Sprintf("%d 0 R", id))
	}
	kids.WriteString("]")
	write(fmt.Sprintf("%d 0 obj<< /Type /Pages /Kids %s /Count %d >>endobj\n", pagesID, kids.String(), len(pageIDs)))

	offsets[f1ID] = out.Len()
	write(fmt.Sprintf("%d 0 obj<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>endobj\n", f1ID))
	offsets[f2ID] = out.Len()
	write(fmt.Sprintf("%d 0 obj<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>endobj\n", f2ID))

	if logoObjNum > 0 {
		iw, ih, _ := LogoConfig(tmpl.Logo)
		offsets[logoObjNum] = out.Len()
		write(fmt.Sprintf("%d 0 obj<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>stream\n",
			logoObjNum, iw, ih, len(tmpl.Logo)))
		out.Write(tmpl.Logo)
		write("\nendstream\nendobj\n")
	}

	for i := range built {
		res := fmt.Sprintf("/Font<< /F1 %d 0 R /F2 %d 0 R >>", f1ID, f2ID)
		if logoObjNum > 0 {
			res += fmt.Sprintf(" /XObject<< /Im1 %d 0 R >>", logoObjNum)
		}
		offsets[pageIDs[i]] = out.Len()
		write(fmt.Sprintf("%d 0 obj<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] /Contents %d 0 R /Resources<< %s >> >>endobj\n",
			pageIDs[i], pagesID, pageW, pageH, contentIDs[i], res))

		stream := built[i].stream
		offsets[contentIDs[i]] = out.Len()
		write(fmt.Sprintf("%d 0 obj<< /Length %d >>stream\n", contentIDs[i], len(stream)))
		out.Write(stream)
		write("\nendstream\nendobj\n")
	}

	xref := out.Len()
	nObj := nextID - 1
	write(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", nObj+1))
	for i := 1; i <= nObj; i++ {
		write(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	write(fmt.Sprintf("trailer<< /Size %d /Root %d 0 R >>\n", nObj+1, catalogID))
	write(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xref))
	return out.Bytes(), nil
}

func collectLineRows(inv model.Invoice) []lineRow {
	b := inv.Bill()
	if b == nil {
		return nil
	}
	out := make([]lineRow, 0, len(b.Lines))
	for i, line := range b.Lines {
		name, price, sum := "", "", ""
		if line.Item != nil {
			name = line.Item.Name
			if line.Item.Price != nil {
				price = line.Item.Price.String()
			}
		}
		if line.Total != nil {
			sum = line.Total.String()
		} else if line.Sum != nil {
			sum = line.Sum.String()
		}
		out = append(out, lineRow{
			pos:   i + 1,
			name:  name,
			qty:   line.Quantity.String(),
			price: price,
			sum:   sum,
		})
	}
	return out
}

func paginateRows(rows []lineRow) [][]lineRow {
	if len(rows) == 0 {
		return [][]lineRow{{}}
	}
	var pages [][]lineRow
	// Page 1: table from y=560 down to ~160 (totals room) → ~25 rows; hard max 34.
	page1Fit := rowsThatFit(560, 160)
	n := page1Fit
	if n > len(rows) {
		n = len(rows)
	}
	pages = append(pages, rows[:n])
	rest := rows[n:]
	for len(rest) > 0 {
		// Continuation: table from ~740 down to 160 (or 100 if not last — we don't know yet).
		fit := rowsThatFit(740, 160)
		if fit > len(rest) {
			fit = len(rest)
		}
		pages = append(pages, rest[:fit])
		rest = rest[fit:]
	}
	return pages
}

func rowsThatFit(topY, bottomY float64) int {
	n := int((topY - bottomY) / rowH)
	if n > maxRowsPage {
		n = maxRowsPage
	}
	if n < 1 {
		n = 1
	}
	return n
}

func buildPageStream(inv model.Invoice, tmpl Template, title string, rows []lineRow, pageN, pageTotal int, last bool, logoW, logoHpt float64) []byte {
	var b bytes.Buffer
	w := func(s string) { b.WriteString(s) }

	accentFill := func() {
		if tmpl.HasAccent {
			w(fmt.Sprintf("%.3f %.3f %.3f rg\n",
				float64(tmpl.Accent[0])/255, float64(tmpl.Accent[1])/255, float64(tmpl.Accent[2])/255))
		} else {
			w("0.122 0.161 0.216 rg\n") // #1F2937
		}
	}
	accentStroke := func() {
		if tmpl.HasAccent {
			w(fmt.Sprintf("%.3f %.3f %.3f RG\n",
				float64(tmpl.Accent[0])/255, float64(tmpl.Accent[1])/255, float64(tmpl.Accent[2])/255))
		} else {
			w("0.5 0.5 0.5 RG\n")
		}
	}

	// Header text
	if tmpl.HeaderText != "" {
		textAt(&b, "/F1", 10, marginL, 795, tmpl.HeaderText)
	}

	// Logo top-right
	if tmpl.HasLogo && logoW > 0 {
		x := pageW - marginR - logoW
		y := 747.0
		if logoHpt < logoH {
			y = 747 + (logoH - logoHpt)
		}
		w(fmt.Sprintf("q %.2f 0 0 %.2f %.2f %.2f cm /Im1 Do Q\n", logoW, logoHpt, x, y))
	}

	// Page marker on multi-page
	if pageTotal > 1 {
		marker := fmt.Sprintf("Seite %d/%d · Page %d/%d", pageN, pageTotal, pageN, pageTotal)
		textAtRight(&b, "/F1", 8, pageW-marginR, 810, marker)
	}

	tableTop := 560.0
	if pageN == 1 {
		drawSellerBuyerMeta(&b, inv, tmpl, title)
	} else {
		tableTop = 740
	}

	// Table header bar
	headerH := 14.0
	barY := tableTop - headerH
	accentFill()
	w(fmt.Sprintf("%.1f %.1f %.1f %.1f re f\n", marginL, barY, pageW-marginL-marginR, headerH))
	w("1 1 1 rg\n")
	textAt(&b, "/F1", 9, 44, barY+3, "Pos")
	textAt(&b, "/F1", 9, 70, barY+3, "Bezeichnung / Item")
	textAtRight(&b, "/F1", 9, 370, barY+3, "Menge")
	textAtRight(&b, "/F1", 9, 450, barY+3, "Preis")
	textAtRight(&b, "/F1", 9, 555, barY+3, "Summe")

	y := barY - 2
	for i, row := range rows {
		y -= rowH
		if i%2 == 1 {
			w("0.96 g\n")
			w(fmt.Sprintf("%.1f %.1f %.1f %.1f re f\n", marginL, y, pageW-marginL-marginR, rowH))
		}
		w("0 0 0 rg\n")
		nameLines := wrapName(row.name, 42, 2)
		textAt(&b, "/F1", 10, 44, y+4, fmt.Sprintf("%d", row.pos))
		textAt(&b, "/F1", 10, 70, y+4, nameLines[0])
		if len(nameLines) > 1 {
			textAt(&b, "/F1", 10, 70, y+4-11, nameLines[1])
		}
		textAtRight(&b, "/F1", 10, 370, y+4, row.qty)
		textAtRight(&b, "/F1", 10, 450, y+4, row.price)
		textAtRight(&b, "/F1", 10, 555, y+4, row.sum)
	}

	if last {
		y -= 24
		drawTotals(&b, inv, y)
	}

	// Footer
	footerY := 36.0
	accentStroke()
	w("0.5 w\n")
	w(fmt.Sprintf("%.1f %.1f m %.1f %.1f l S\n", marginL, footerY+18, pageW-marginR, footerY+18))
	w("0 0 0 rg\n")
	if tmpl.FooterText != "" {
		flines := splitFooter(tmpl.FooterText, 110)
		fy := footerY + 8
		for i := len(flines) - 1; i >= 0; i-- {
			textAt(&b, "/F1", 8, marginL, fy, flines[i])
			fy += 10
		}
	}

	return b.Bytes()
}

func drawSellerBuyerMeta(b *bytes.Buffer, inv model.Invoice, tmpl Template, title string) {
	bill := inv.Bill()

	// Seller (left)
	y := 740.0
	textAt(b, "/F2", 11, marginL, y, inv.VendorName())
	y -= 14
	if bill != nil && bill.Supplier != nil && len(bill.Supplier.Addresses) > 0 {
		y = drawAddress(b, bill.Supplier.Addresses[0], y)
	}
	if tid := inv.SellerTaxID(); tid != "" {
		textAt(b, "/F1", 9, marginL, y, "USt-IdNr: "+tid)
		y -= 12
	}
	_ = y

	// Buyer (left)
	y = 620
	textAt(b, "/F1", 9, marginL, y, "Bill to / Rechnungsempfaenger")
	y -= 14
	textAt(b, "/F2", 10, marginL, y, inv.BuyerName())
	y -= 14
	if bill != nil && bill.Customer != nil && len(bill.Customer.Addresses) > 0 {
		y = drawAddress(b, bill.Customer.Addresses[0], y)
	}
	if tid := inv.BuyerTaxID(); tid != "" {
		textAt(b, "/F1", 9, marginL, y, "USt-IdNr: "+tid)
	}

	// Meta (right)
	metaX := 340.0
	y = 740
	if tmpl.HasAccent {
		b.WriteString(fmt.Sprintf("%.3f %.3f %.3f rg\n",
			float64(tmpl.Accent[0])/255, float64(tmpl.Accent[1])/255, float64(tmpl.Accent[2])/255))
	} else {
		b.WriteString("0 0 0 rg\n")
	}
	textAt(b, "/F2", 16, metaX, y, title)
	b.WriteString("0 0 0 rg\n")
	y -= 20
	textAt(b, "/F1", 10, metaX, y, "Number: "+inv.InvoiceNumber())
	y -= 14
	textAt(b, "/F1", 10, metaX, y, "Date: "+inv.InvoiceDate())
	y -= 14
	textAt(b, "/F1", 10, metaX, y, "TypeCode: "+inv.TypeCode())
	y -= 14
	textAt(b, "/F1", 10, metaX, y, "Currency: "+inv.Currency())
}

func drawAddress(b *bytes.Buffer, a *org.Address, y float64) float64 {
	if a == nil {
		return y
	}
	if a.Street != "" {
		textAt(b, "/F1", 9, marginL, y, a.Street)
		y -= 12
	}
	loc := strings.TrimSpace(string(a.Code) + " " + a.Locality)
	if loc != "" {
		textAt(b, "/F1", 9, marginL, y, loc)
		y -= 12
	}
	if a.Country != "" {
		textAt(b, "/F1", 9, marginL, y, string(a.Country))
		y -= 12
	}
	return y
}

func drawTotals(b *bytes.Buffer, inv model.Invoice, y float64) {
	xLabel := 380.0
	xVal := 555.0
	bill := inv.Bill()
	netto, gesamt := "0.00", "0.00"
	if bill != nil && bill.Totals != nil {
		netto = bill.Totals.Total.String()
		gesamt = bill.Totals.TotalWithTax.String()
	}
	textAt(b, "/F1", 10, xLabel, y, "Netto / Subtotal")
	textAtRight(b, "/F1", 10, xVal, y, netto)
	y -= 14
	for _, bucket := range inv.VATBreakdown() {
		label := "USt"
		pct := bucket.Percent
		if pct == "" {
			pct = bucket.Rate
		}
		if pct != "" {
			pct = strings.TrimSpace(strings.TrimSuffix(pct, "%"))
			label = fmt.Sprintf("USt %s %%", pct)
		}
		textAt(b, "/F1", 10, xLabel, y, label)
		textAtRight(b, "/F1", 10, xVal, y, fmt.Sprintf("%.2f", model.CentsToFloat(bucket.VAT)))
		y -= 14
	}
	textAt(b, "/F2", 10, xLabel, y, "Gesamt / Total")
	textAtRight(b, "/F2", 10, xVal, y, gesamt)
	y -= 16
	// Signed payable — credit notes keep "Payable: -N.NN" (ph8_test asserts).
	payable := model.CentsToFloat(inv.TotalCents())
	textAt(b, "/F2", 11, xLabel, y, fmt.Sprintf("Payable: %.2f %s", payable, inv.Currency()))
}

func wrapName(s string, width, maxLines int) []string {
	s = ascii(s)
	if s == "" {
		return []string{""}
	}
	var lines []string
	for len(s) > 0 && len(lines) < maxLines {
		if len(s) <= width {
			lines = append(lines, s)
			break
		}
		cut := width
		if len(lines) == maxLines-1 {
			if len(s) > width {
				lines = append(lines, s[:width-3]+"...")
			} else {
				lines = append(lines, s)
			}
			break
		}
		// Prefer break at space.
		sp := strings.LastIndex(s[:cut], " ")
		if sp > width/3 {
			cut = sp
		}
		lines = append(lines, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func splitFooter(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		para = ascii(para)
		for len(para) > width {
			out = append(out, para[:width])
			para = para[width:]
		}
		out = append(out, para)
	}
	return out
}

func textAt(b *bytes.Buffer, font string, size float64, x, y float64, s string) {
	b.WriteString("BT\n")
	b.WriteString(fmt.Sprintf("%s %.1f Tf\n", font, size))
	b.WriteString(fmt.Sprintf("%.1f %.1f Td\n", x, y))
	b.WriteString("(")
	b.WriteString(pdfEscape(s))
	b.WriteString(") Tj\nET\n")
}

func textAtRight(b *bytes.Buffer, font string, size float64, rightX, y float64, s string) {
	// Approximate Helvetica width: 0.5 * size * len for ASCII.
	w := 0.5 * size * float64(len(ascii(s)))
	textAt(b, font, size, rightX-w, y, s)
}

func pdfEscape(s string) string {
	s = ascii(s)
	repl := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)")
	return repl.Replace(s)
}

func ascii(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ä', 'Ä':
			b.WriteString("ae")
		case 'ö', 'Ö':
			b.WriteString("oe")
		case 'ü', 'Ü':
			b.WriteString("ue")
		case 'ß':
			b.WriteString("ss")
		default:
			if r < 32 || r > 126 {
				b.WriteByte('?')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

