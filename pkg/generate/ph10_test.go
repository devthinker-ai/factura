package generate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/validate"
)

// optionalFieldsJSON mirrors frontend toGobl() with Phase 10 optional EN 16931
// fields filled (project/contract/order/delivery/period/identities/ref/discounts).
// Proves the optional surface does not break the mandatory self-check.
func optionalFieldsJSON() []byte {
	raw := consoleShapeJSON("standard", "R-OPT-1", nil)
	var inv map[string]any
	if err := json.Unmarshal(raw, &inv); err != nil {
		panic(err)
	}

	inv["ordering"] = map[string]any{
		"code": "PO-42",
		"projects": []map[string]string{
			{"code": "PRJ-1"},
		},
		"contracts": []map[string]string{
			{"code": "CTR-9"},
		},
		"purchases": []map[string]string{
			{"code": "PO-42"},
		},
		"sales": []map[string]string{
			{"code": "SO-7"},
		},
		"tender": []map[string]string{
			{"code": "TND-3"},
		},
		"cost": "1287:65464",
		"period": map[string]string{
			"start": "2026-08-01",
			"end":   "2026-08-31",
		},
	}
	// BT-72: gobl.cii maps Delivery.Date (json "date"), not receive_date.
	inv["delivery"] = map[string]string{"date": "2026-08-15"}

	supplier := inv["supplier"].(map[string]any)
	supplier["identities"] = []map[string]string{
		{"code": "SUP-99"},
		{"scope": "legal", "code": "HRB 12345"},
	}
	customer := inv["customer"].(map[string]any)
	customer["identities"] = []map[string]string{
		{"code": "CUST-7"},
	}

	pay := inv["payment"].(map[string]any)
	instr := pay["instructions"].(map[string]any)
	instr["ref"] = "R-OPT-1 / PO-42"
	pay["terms"] = map[string]any{
		"due_dates": []map[string]string{{"date": "2026-09-30"}},
		"notes":     "Skonto 2% binnen 10 Tagen",
	}

	inv["discounts"] = []map[string]any{{
		"reason":  "Promo",
		"percent": "10%",
		"base":    "100.00",
		"taxes":   []map[string]string{{"cat": "VAT", "rate": "general"}},
	}}

	b, err := json.Marshal(inv)
	if err != nil {
		panic(err)
	}
	return b
}

func TestPh10_OptionalFieldsPassSelfCheck(t *testing.T) {
	inv, err := model.FromJSON(optionalFieldsJSON())
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
	cii, err := os.ReadFile(xr)
	if err != nil {
		t.Fatal(err)
	}
	s := string(cii)
	needles := []string{
		"PRJ-1",          // BT-11
		"CTR-9",          // BT-12
		"PO-42",          // BT-13 / buyer ref
		"SO-7",           // BT-14
		"TND-3",          // BT-17
		"1287:65464",     // BT-19
		"20260815",       // BT-72 delivery date
		"20260801",       // BT-73
		"20260831",       // BT-74
		"SUP-99",         // BT-29
		"CUST-7",         // BT-46
		"HRB 12345",      // BT-30
		"R-OPT-1 / PO-42", // BT-83
		"Skonto 2%",      // BT-20
		"Promo",          // BT-107 reason
	}
	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Fatalf("CII missing %q", n)
		}
	}
	if _, err := os.Stat(zf); err != nil {
		t.Fatalf("hybrid PDF missing: %v", err)
	}
}
