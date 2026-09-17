// Package peppol is Factura's Peppol Service Provider layer: participant
// registry types, BIS Billing 3.0 SBD envelope, and a swappable AccessPoint.
//
// Architecture (GTM.md §8): Factura is the SP; ONE Access Point adapter carries
// documents. Parse/validate/generate stay offline — this package is the only
// online path in the binary.
package peppol

import (
	"context"
	"time"
)

// Service types for a Peppol participant.
const (
	ServiceBuyer       = "buyer"
	ServiceSeller      = "seller"
	ServiceBuyerSeller = "buyer-seller"
)

// Direction of a network message.
const (
	DirectionIn  = "in"
	DirectionOut = "out"
)

// Receipt / message status values.
const (
	StatusSending   = "sending"
	StatusAccepted  = "accepted"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Peppol BIS Billing 3.0 URIs (pinned).
const (
	CollaborationProtocol = "urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	ProcessURI            = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
	PayloadMimeXML        = "text/xml"
	PayloadContentType    = "application/xml"
)

// Participant is a Peppol-identified business.
type Participant struct {
	ID          string `json:"peppol_id"`
	ServiceType string `json:"service_type"` // buyer|seller|buyer-seller
	Name        string `json:"name"`
	IsSelf      bool   `json:"is_self"`
}

// PartyID identifies a participant inside an SBD envelope.
type PartyID struct {
	Identifier     string `json:"identifier"`
	ServicePointURL string `json:"service_point_url,omitempty"`
}

// Control is the SBD message:control block (routing + timestamps).
type Control struct {
	Sender        string    `json:"sender,omitempty"`
	Receiver      string    `json:"receiver,omitempty"`
	ServicePoint  string    `json:"service_point,omitempty"`
	CreationTime  time.Time `json:"creation_time"`
	APMessageID   string    `json:"ap_message_id,omitempty"`
	Status        string    `json:"status,omitempty"` // accepted|delivered|failed on receipts
}

// BisMessage is a Peppol BIS Billing 3.0 Standard Business Document envelope
// wrapping an EN 16931 invoice (XRechnung CII/UBL XML).
type BisMessage struct {
	CollaborationProtocol string
	Process               string
	Sender                PartyID
	Receiver              PartyID
	Payload               []byte // exact invoice XML bytes — never re-serialized
	PayloadMime           string
	Control               Control
}

// Receipt is the AP delivery proof (GoBD evidence when archived).
type Receipt struct {
	APMessageID string
	Status      string // accepted|delivered|failed
	Raw         []byte
	At          time.Time
}

// AccessPoint is the swappable carrier seam. Exactly one concrete adapter is
// wired at serve time; the rest of Factura never names the vendor.
type AccessPoint interface {
	Name() string
	Send(ctx context.Context, m BisMessage) (Receipt, error)
	// Poll returns new inbound BIS messages (for APs without push). Idempotent.
	Poll(ctx context.Context, since time.Time) ([]BisMessage, error)
	Register(ctx context.Context, p Participant) (endpoint string, err error)
	// Ping checks AP reachability (settings "test connection").
	Ping(ctx context.Context) error
}

// Config is env-driven AP wiring (no config file).
type Config struct {
	AP          string // FACTURA_PEPPOL_AP: "fake" | "http" (default "http")
	APURL       string // FACTURA_PEPPOL_AP_URL
	APKey       string // FACTURA_PEPPOL_AP_KEY
	SelfID      string // FACTURA_PEPPOL_SELF_ID
	CallbackURL string // FACTURA_PEPPOL_CALLBACK_URL
}
