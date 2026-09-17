package peppol_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devthinker-ai/factura/pkg/peppol"
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
	t.Fatalf("testdata/%s not found", name)
	return nil
}

func TestEnvelopeRoundTrip(t *testing.T) {
	payload := readTD(t, "xrechnung-302-cii.xml")
	m := peppol.BisMessage{
		CollaborationProtocol: peppol.CollaborationProtocol,
		Process:               peppol.ProcessURI,
		Sender:                peppol.PartyID{Identifier: "DE:SENDER", ServicePointURL: "https://sp.example/as4"},
		Receiver:              peppol.PartyID{Identifier: "DE:RECEIVER"},
		Payload:               payload,
		Control: peppol.Control{
			APMessageID:  "msg-roundtrip-1",
			CreationTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
	xml, err := m.ToXML()
	if err != nil {
		t.Fatal(err)
	}
	got, err := peppol.FromXML(xml)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sender.Identifier != "DE:SENDER" || got.Receiver.Identifier != "DE:RECEIVER" {
		t.Fatalf("participants: %+v / %+v", got.Sender, got.Receiver)
	}
	if got.CollaborationProtocol != peppol.CollaborationProtocol {
		t.Fatalf("protocol=%q", got.CollaborationProtocol)
	}
	if got.Process != peppol.ProcessURI {
		t.Fatalf("process=%q", got.Process)
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Fatalf("payload mismatch: got %d want %d bytes", len(got.Payload), len(payload))
	}
	if got.Control.APMessageID != "msg-roundtrip-1" {
		t.Fatalf("ap id=%q", got.Control.APMessageID)
	}
}

func TestEnvelopeFromGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "bis-billing-3.0-golden.xml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := peppol.FromXML(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("<Invoice xmlns='urn:test'>golden</Invoice>")
	if !bytes.Equal(got.Payload, want) {
		t.Fatalf("payload=%q", got.Payload)
	}
	if got.Sender.Identifier != "DE:SENDER" || got.Control.APMessageID != "golden-fixture-1" {
		t.Fatalf("%+v", got)
	}
}

func TestFakeSendDelivered(t *testing.T) {
	f := peppol.NewFake()
	rec, err := f.Send(t.Context(), peppol.BisMessage{
		Sender:   peppol.PartyID{Identifier: "DE:A"},
		Receiver: peppol.PartyID{Identifier: "DE:B"},
		Payload:  []byte("<x/>"),
		Control:  peppol.Control{APMessageID: "m1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != peppol.StatusDelivered || rec.APMessageID != "m1" {
		t.Fatalf("%+v", rec)
	}
	if len(f.Sent) != 1 {
		t.Fatalf("sent=%d", len(f.Sent))
	}
}
