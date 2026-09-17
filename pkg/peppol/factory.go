package peppol

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/devthinker-ai/factura/pkg/peppol/ap_http"
)

// NewAccessPoint builds the configured AccessPoint from Config / env.
// FACTURA_PEPPOL_AP: "fake" | "http" (default "http").
// The AP vendor name lives only inside the adapter package.
func NewAccessPoint(cfg Config) (AccessPoint, error) {
	cfg = cfg.withEnvDefaults()
	name := strings.ToLower(strings.TrimSpace(cfg.AP))
	if name == "" {
		name = "http"
	}
	switch name {
	case "fake":
		return NewFake(), nil
	case "http":
		ap, err := ap_http.New(ap_http.Config{
			BaseURL:     cfg.APURL,
			APIKey:      cfg.APKey,
			SelfID:      cfg.SelfID,
			CallbackURL: cfg.CallbackURL,
		})
		if err != nil {
			return nil, err
		}
		return adaptHTTP(ap), nil
	default:
		return nil, fmt.Errorf("peppol: unknown FACTURA_PEPPOL_AP %q (want fake|http)", cfg.AP)
	}
}

// ConfigFromEnv reads Peppol env vars.
func ConfigFromEnv() Config {
	return Config{}.withEnvDefaults()
}

func (c Config) withEnvDefaults() Config {
	if c.AP == "" {
		c.AP = os.Getenv("FACTURA_PEPPOL_AP")
	}
	if c.APURL == "" {
		c.APURL = os.Getenv("FACTURA_PEPPOL_AP_URL")
	}
	if c.APKey == "" {
		c.APKey = os.Getenv("FACTURA_PEPPOL_AP_KEY")
	}
	if c.SelfID == "" {
		c.SelfID = os.Getenv("FACTURA_PEPPOL_SELF_ID")
	}
	if c.CallbackURL == "" {
		c.CallbackURL = os.Getenv("FACTURA_PEPPOL_CALLBACK_URL")
	}
	return c
}

// httpBridge adapts ap_http.Client to peppol.AccessPoint without leaking
// the adapter type into the rest of the engine (adapter name stays in-package).
type httpBridge struct {
	c *ap_http.Client
}

func adaptHTTP(c *ap_http.Client) AccessPoint { return &httpBridge{c: c} }

func (h *httpBridge) Name() string { return h.c.Name() }

func (h *httpBridge) Send(ctx context.Context, m BisMessage) (Receipt, error) {
	sbd, err := m.ToXML()
	if err != nil {
		return Receipt{}, err
	}
	msgID := m.Control.APMessageID
	r, err := h.c.SendSBD(ctx, msgID, sbd)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{
		APMessageID: r.APMessageID,
		Status:      r.Status,
		Raw:         r.Raw,
		At:          r.At,
	}, nil
}

func (h *httpBridge) Poll(ctx context.Context, since time.Time) ([]BisMessage, error) {
	msgs, err := h.c.Poll(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]BisMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, BisMessage{
			CollaborationProtocol: firstNonEmpty(m.CollaborationProtocol, CollaborationProtocol),
			Process:               firstNonEmpty(m.Process, ProcessURI),
			Sender:                PartyID{Identifier: m.SenderID},
			Receiver:              PartyID{Identifier: m.ReceiverID},
			Payload:               m.Payload,
			Control: Control{
				APMessageID:  m.APMessageID,
				CreationTime: m.CreationTime,
				Sender:       m.SenderID,
				Receiver:     m.ReceiverID,
			},
		})
	}
	return out, nil
}

func (h *httpBridge) Register(ctx context.Context, p Participant) (string, error) {
	return h.c.Register(ctx, ap_http.Participant{
		ID:          p.ID,
		ServiceType: p.ServiceType,
		Name:        p.Name,
	})
}

func (h *httpBridge) Ping(ctx context.Context) error {
	return h.c.Ping(ctx)
}
