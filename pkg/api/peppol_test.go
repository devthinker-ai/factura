package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/license"
	"github.com/devthinker-ai/factura/pkg/peppol"
)

func newPeppolServer(t *testing.T) (*api.Server, *archive.Store, *peppol.Fake) {
	t.Helper()
	peppol.SetRetryBackoffsForTest([]time.Duration{time.Millisecond, time.Millisecond, time.Millisecond})
	t.Cleanup(func() {
		peppol.SetRetryBackoffsForTest([]time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second})
	})
	srv, store := newTestServer(t)
	// Phase 4 suite runs under Pro — Peppol is gated for Solo (Phase 6).
	pro := license.CapsForPlan(license.PlanPro)
	pro.Licensed = true
	srv.ApplyCaps(pro)
	fake := peppol.NewFake()
	eng := &peppol.Engine{
		Store:  store,
		AP:     fake,
		SelfID: "DE:SELF123",
	}
	srv.SetPeppol(eng)
	_ = store.AddParticipant(archive.Participant{
		PeppolID: "DE:SELF123", ServiceType: "buyer-seller", Name: "Self GmbH", IsSelf: true,
	})
	_ = store.AddParticipant(archive.Participant{
		PeppolID: "DE:BUYER456", ServiceType: "buyer", Name: "Buyer AG",
	})
	return srv, store, fake
}

func wrapBIS(t *testing.T, payload []byte, sender, receiver, apMsgID string) []byte {
	t.Helper()
	m := peppol.BisMessage{
		CollaborationProtocol: peppol.CollaborationProtocol,
		Process:               peppol.ProcessURI,
		Sender:                peppol.PartyID{Identifier: sender},
		Receiver:              peppol.PartyID{Identifier: receiver},
		Payload:               payload,
		Control: peppol.Control{
			Sender:       sender,
			Receiver:     receiver,
			CreationTime: time.Now().UTC(),
			APMessageID:  apMsgID,
		},
	}
	xml, err := m.ToXML()
	if err != nil {
		t.Fatal(err)
	}
	return xml
}

func TestPeppolReceiveCallback(t *testing.T) {
	srv, store, _ := newPeppolServer(t)
	payload := readTD(t, "xrechnung-302-cii.xml")
	sbd := wrapBIS(t, payload, "DE:SUPPLIER", "DE:SELF123", "ap-in-1")

	req := httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader(sbd))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["status"] != "valid" {
		t.Fatalf("got=%v", got)
	}
	invID := int64(got["invoice_id"].(float64))
	row, err := store.Get(invID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Source != "peppol:receive" {
		t.Fatalf("source=%q", row.Source)
	}
	msgs, err := store.ListMessages(archive.PeppolDirIn, "", "", 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs=%v err=%v", msgs, err)
	}
	if msgs[0].Status != archive.PeppolDelivered || msgs[0].InvoiceID == nil || *msgs[0].InvoiceID != invID {
		t.Fatalf("msg=%+v", msgs[0])
	}

	// Idempotent re-POST
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader(sbd)))
	if rr2.Code != http.StatusOK {
		t.Fatalf("repost status=%d", rr2.Code)
	}
	msgs2, _ := store.ListMessages(archive.PeppolDirIn, "", "", 10)
	if len(msgs2) != 1 {
		t.Fatalf("dup rows: %d", len(msgs2))
	}
	invCount, _ := store.Count(archive.Filter{})
	if invCount != 1 {
		t.Fatalf("invoice rows=%d", invCount)
	}
}

func TestPeppolReceiveBadEnvelope(t *testing.T) {
	srv, store, _ := newPeppolServer(t)
	req := httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader([]byte("not-xml")))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d", rr.Code)
	}
	msgs, _ := store.ListMessages("", archive.PeppolFailed, "", 10)
	if len(msgs) != 1 || len(msgs[0].BisXML) == 0 {
		t.Fatalf("expected failed message with raw: %+v", msgs)
	}
}

func TestPeppolReceiveInvalidInvoice(t *testing.T) {
	srv, _, _ := newPeppolServer(t)
	payload := readTD(t, "broken-amounts.xml")
	sbd := wrapBIS(t, payload, "DE:SUPPLIER", "DE:SELF123", "ap-in-invalid")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader(sbd)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["status"] != "invalid" {
		t.Fatalf("got=%v", got)
	}
}

func TestPeppolReceivePoll(t *testing.T) {
	srv, store, fake := newPeppolServer(t)
	payload := readTD(t, "xrechnung-302-cii.xml")
	fake.InjectInbound(
		peppol.BisMessage{
			Sender: peppol.PartyID{Identifier: "DE:A"}, Receiver: peppol.PartyID{Identifier: "DE:SELF123"},
			Payload: payload, Control: peppol.Control{APMessageID: "poll-1", CreationTime: time.Now().UTC()},
		},
		peppol.BisMessage{
			Sender: peppol.PartyID{Identifier: "DE:B"}, Receiver: peppol.PartyID{Identifier: "DE:SELF123"},
			Payload: payload, Control: peppol.Control{APMessageID: "poll-2", CreationTime: time.Now().UTC()},
		},
	)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/peppol/inbound", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	news, _ := got["new"].([]any)
	if len(news) != 2 {
		t.Fatalf("new=%v", got)
	}
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/peppol/inbound", nil))
	_ = json.Unmarshal(rr2.Body.Bytes(), &got)
	news, _ = got["new"].([]any)
	if len(news) != 0 {
		t.Fatalf("second poll not empty: %v", got)
	}
	n, _ := store.Count(archive.Filter{})
	if n != 2 {
		t.Fatalf("invoices=%d", n)
	}
}

func TestPeppolSendHappy(t *testing.T) {
	srv, store, _ := newPeppolServer(t)
	payload := readTD(t, "xrechnung-302-cii.xml")
	row, err := store.Ingest(payload, "send-src-1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"invoice_id": row.ID, "to": "DE:BUYER456"})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["status"] != "delivered" {
		t.Fatalf("%v", got)
	}
	msgID := int64(got["message_id"].(float64))
	msg, err := store.MessageByID(msgID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Status != archive.PeppolDelivered || len(msg.Receipt) == 0 || len(msg.BisXML) == 0 {
		t.Fatalf("gobd incomplete: %+v receipt=%d bis=%d", msg, len(msg.Receipt), len(msg.BisXML))
	}
	// GoBD: bis round-trips to exact payload
	bis, err := peppol.FromXML(msg.BisXML)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bis.Payload, payload) {
		t.Fatalf("bis payload mismatch")
	}
	invID := int64(got["invoice_id"].(float64))
	outRow, err := store.Get(invID)
	if err != nil {
		t.Fatal(err)
	}
	if outRow.Source != "peppol:send" {
		t.Fatalf("source=%q", outRow.Source)
	}
}

func TestPeppolSendValidationGate(t *testing.T) {
	srv, store, fake := newPeppolServer(t)
	payload := readTD(t, "broken-amounts.xml")
	row, err := store.Ingest(payload, "bad-inv")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"invoice_id": row.ID, "to": "DE:BUYER456"})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader(body)))
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	msgs, _ := store.ListMessages(archive.PeppolDirOut, "", "", 10)
	if len(msgs) != 0 {
		t.Fatalf("unexpected message rows: %d", len(msgs))
	}
	if fake.SendCount != 0 {
		t.Fatalf("ap send count=%d", fake.SendCount)
	}
}

func TestPeppolSendAPRejectionAndRetry(t *testing.T) {
	srv, store, fake := newPeppolServer(t)
	fake.RejectNext = 1
	payload := readTD(t, "xrechnung-302-cii.xml")
	row, _ := store.Ingest(payload, "rej-1")
	body, _ := json.Marshal(map[string]any{"invoice_id": row.ID, "to": "DE:BUYER456"})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader(body)))
	if rr.Code != 424 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["error"] != "ap rejected" {
		t.Fatalf("%v", got)
	}
	msgID := int64(got["message_id"].(float64))
	msg, _ := store.MessageByID(msgID)
	if msg.Status != archive.PeppolFailed || len(msg.Receipt) == 0 {
		t.Fatalf("%+v", msg)
	}

	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/peppol/send/"+strconv.FormatInt(msgID, 10)+"/retry", nil))
	if rr2.Code != http.StatusCreated {
		t.Fatalf("retry status=%d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestPeppolSendAPDown(t *testing.T) {
	srv, store, fake := newPeppolServer(t)
	fake.FailNext = 3
	payload := readTD(t, "xrechnung-302-cii.xml")
	row, _ := store.Ingest(payload, "down-1")
	body, _ := json.Marshal(map[string]any{"invoice_id": row.ID, "to": "DE:BUYER456"})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader(body)))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if fake.SendCount != 3 {
		t.Fatalf("attempts=%d want 3", fake.SendCount)
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	msgID := int64(got["message_id"].(float64))
	msg, _ := store.MessageByID(msgID)
	if msg.Status != archive.PeppolFailed {
		t.Fatalf("status=%s", msg.Status)
	}

	// Recover and retry
	fake.FailNext = 0
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/peppol/send/"+strconv.FormatInt(msgID, 10)+"/retry", nil))
	if rr2.Code != http.StatusCreated {
		t.Fatalf("retry status=%d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestPeppolParticipantsAndSelfReceive(t *testing.T) {
	srv, store, _ := newPeppolServer(t)
	ps, _ := store.Participants()
	if len(ps) < 2 {
		t.Fatalf("participants=%d", len(ps))
	}
	self, err := store.Self()
	if err != nil || self.PeppolID != "DE:SELF123" {
		t.Fatalf("self=%+v err=%v", self, err)
	}

	payload := readTD(t, "xrechnung-302-cii.xml")
	// sender == self → receive:self
	sbd := wrapBIS(t, payload, "DE:SELF123", "DE:SELF123", "self-bounce-1")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/peppol/inbound", bytes.NewReader(sbd)))
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	row, _ := store.Get(int64(got["invoice_id"].(float64)))
	if row.Source != "peppol:receive:self" {
		t.Fatalf("source=%q", row.Source)
	}

	// unregistered participant → 409
	body, _ := json.Marshal(map[string]any{"invoice_id": row.ID, "to": "DE:UNKNOWN"})
	rr2 := httptest.NewRecorder()
	valid, _ := store.Ingest(readTD(t, "xrechnung-302-cii.xml"), "for-409")
	body, _ = json.Marshal(map[string]any{"invoice_id": valid.ID, "to": "DE:UNKNOWN"})
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/peppol/send", bytes.NewReader(body)))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestPeppolStatusEndpoint(t *testing.T) {
	srv, _, _ := newPeppolServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/peppol/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("%d", rr.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["ap_name"] != "fake" || got["self_id"] != "DE:SELF123" {
		t.Fatalf("%v", got)
	}
}
