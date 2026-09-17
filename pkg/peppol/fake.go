package peppol

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory AccessPoint for tests and `serve --peppol-fake`.
// All engine tests run against Fake — zero network sockets.
type Fake struct {
	mu sync.Mutex

	name string

	// Outbound
	Sent       []BisMessage
	SendCount  int
	FailNext   int    // next N Send calls return error (counts attempts)
	RejectNext int    // next N Send calls return StatusFailed receipt
	ForceStatus string // if set, every successful Send returns this status
	ReceiptRaw []byte

	// Inbound poll queue
	inbound   []BisMessage
	polledIDs map[string]bool

	// Register
	Registered []Participant
	Endpoint   string

	// Ping
	PingErr error
}

// NewFake returns a ready Fake AccessPoint.
func NewFake() *Fake {
	return &Fake{
		name:      "fake",
		polledIDs: map[string]bool{},
		Endpoint:  "https://fake.peppol.local/as4",
		ReceiptRaw: []byte(`{"status":"delivered"}`),
	}
}

func (f *Fake) Name() string { return f.name }

// InjectInbound queues messages for Poll (and optional push tests).
func (f *Fake) InjectInbound(msgs ...BisMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inbound = append(f.inbound, msgs...)
}

// ResetClearsSent clears outbound history (tests).
func (f *Fake) ResetClearsSent() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = nil
	f.SendCount = 0
}

func (f *Fake) Send(ctx context.Context, m BisMessage) (Receipt, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SendCount++
	if f.FailNext > 0 {
		f.FailNext--
		return Receipt{}, fmt.Errorf("peppol/fake: ap unreachable (attempt %d)", f.SendCount)
	}
	msgID := m.Control.APMessageID
	if msgID == "" {
		msgID = fmt.Sprintf("fake-%d", time.Now().UnixNano())
	}
	status := StatusDelivered
	if f.RejectNext > 0 {
		f.RejectNext--
		status = StatusFailed
	} else if f.ForceStatus != "" {
		status = f.ForceStatus
	}
	raw := f.ReceiptRaw
	if raw == nil {
		raw = []byte(`{"status":"` + status + `"}`)
	}
	cp := m
	cp.Control.APMessageID = msgID
	cp.Control.Status = status
	f.Sent = append(f.Sent, cp)
	return Receipt{
		APMessageID: msgID,
		Status:      status,
		Raw:         append([]byte(nil), raw...),
		At:          time.Now().UTC(),
	}, nil
}

func (f *Fake) Poll(ctx context.Context, since time.Time) ([]BisMessage, error) {
	_ = ctx
	_ = since
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []BisMessage
	var remain []BisMessage
	for _, m := range f.inbound {
		id := m.Control.APMessageID
		if id == "" {
			id = fmt.Sprintf("%s-%s-%d", m.Sender.Identifier, m.Receiver.Identifier, m.Control.CreationTime.UnixNano())
			m.Control.APMessageID = id
		}
		if f.polledIDs[id] {
			continue
		}
		f.polledIDs[id] = true
		out = append(out, m)
	}
	// Keep inbound list but mark polled; second poll returns none.
	f.inbound = remain
	return out, nil
}

func (f *Fake) Register(ctx context.Context, p Participant) (string, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Registered = append(f.Registered, p)
	return f.Endpoint, nil
}

func (f *Fake) Ping(ctx context.Context) error {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.PingErr
}
