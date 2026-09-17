package model_test

import (
	"testing"

	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/invopop/gobl/cal"
)

func TestNewDEB2B_MixedRates(t *testing.T) {
	inv, err := model.NewDEB2B("T-1", cal.MakeDate(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if inv.VendorName() == "" || inv.BuyerName() == "" {
		t.Fatal("missing parties")
	}
	if inv.TotalCents() <= 0 {
		t.Fatalf("total=%d", inv.TotalCents())
	}
	buckets := inv.VATBreakdown()
	if len(buckets) < 2 {
		t.Fatalf("want mixed VAT buckets, got %+v", buckets)
	}
	j, err := inv.JSON()
	if err != nil || len(j) < 10 {
		t.Fatalf("json: %v", err)
	}
}
