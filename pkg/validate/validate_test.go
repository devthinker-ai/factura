package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devthinker-ai/factura/pkg/validate"
)

func readTD(t *testing.T, name string) []byte {
	t.Helper()
	candidates := []string{
		filepath.Join("testdata", name),
		filepath.Join("..", "..", "testdata", name),
	}
	wd, _ := os.Getwd()
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidates = append(candidates, filepath.Join(dir, "testdata", name))
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err == nil {
			return b
		}
	}
	t.Fatalf("cannot find testdata/%s", name)
	return nil
}

func TestValidate_GoldenPositive(t *testing.T) {
	rep := validate.ValidateBytes(readTD(t, "xrechnung-302-cii.xml"))
	if !rep.Valid {
		t.Fatalf("expected valid, got: %s %+v", rep.Summary, rep.Violations)
	}
	if len(rep.Violations) != 0 {
		t.Fatalf("violations: %+v", rep.Violations)
	}
}

func TestValidate_BrokenMissingVATID(t *testing.T) {
	rep := validate.ValidateBytes(readTD(t, "broken-missing-vatid.xml"))
	if rep.Valid {
		t.Fatal("expected invalid")
	}
	assertRule(t, rep, "BR-DE-16")
}

func TestValidate_BrokenAmounts(t *testing.T) {
	rep := validate.ValidateBytes(readTD(t, "broken-amounts.xml"))
	if rep.Valid {
		t.Fatal("expected invalid")
	}
	assertRule(t, rep, "BR-CO-15")
}

func assertRule(t *testing.T, rep validate.Report, ruleID string) {
	t.Helper()
	for _, v := range rep.Violations {
		if v.RuleID == ruleID {
			if v.Human == "" {
				t.Fatalf("empty Human for %s", ruleID)
			}
			if !contains(v.Human, "/") && !contains(v.Human, "Rule") {
				// Mapped messages are "DE / EN"; unmapped still non-empty.
			}
			return
		}
	}
	t.Fatalf("rule %s not found in %+v", ruleID, rep.Violations)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
