package validate

import xinvoicepdf "github.com/andeedotnet/go-xinvoice-pdf"

func init() {
	pdfExtract = func(data []byte) ([]byte, string, error) {
		return xinvoicepdf.ExtractXML(data)
	}
}
