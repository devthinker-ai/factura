// Package ap_http is Factura's ONE swappable Peppol Access Point adapter.
//
// adapter: verify-against-real-AP
//
// Choice (AGENTS.md / Phase 4): we ship against a well-specified HTTP contract
// rather than hard-coding a commercial vendor (Storecove / GLC / XRoad) until
// a design partner names the AP. All AP quirks (auth header, message-id,
// callback schema, poll cursor) live HERE — nothing outside this package
// names the carrier.
//
// Expected HTTP API (documented contract — zero new Go deps, stdlib net/http):
//
//	POST {BaseURL}/v1/messages
//	  Authorization: Bearer {APIKey}
//	  Content-Type: application/xml
//	  X-Idempotency-Key: {ap_message_id}
//	  Body: BIS 3.0 SBD XML
//	  202 → {"id":"…","status":"accepted"|"delivered"|"failed"} + optional raw receipt body
//	  5xx / network → retryable (caller applies backoff)
//
//	GET {BaseURL}/v1/messages?since={RFC3339}
//	  → {"messages":[{"id":"…","sbd_xml_base64":"…"}, …]}
//
//	POST {BaseURL}/v1/participants
//	  Body: {"peppol_id","service_type","name","callback_url"}
//	  → {"endpoint":"https://…"}
//
//	GET {BaseURL}/v1/health → 200
package ap_http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the HTTP Access Point contract above.
type Client struct {
	baseURL     string
	apiKey      string
	selfID      string
	callbackURL string
	http        *http.Client
}

// Config for New.
type Config struct {
	BaseURL     string
	APIKey      string
	SelfID      string
	CallbackURL string
}

// Message is the wire-agnostic send unit (mirrors peppol.BisMessage fields
// needed by the HTTP API without importing peppol — avoids import cycles).
type Message struct {
	CollaborationProtocol string
	Process               string
	SenderID              string
	ReceiverID            string
	Payload               []byte
	APMessageID           string
	CreationTime          time.Time
}

// Participant registration payload.
type Participant struct {
	ID          string
	ServiceType string
	Name        string
}

// Receipt from Send.
type Receipt struct {
	APMessageID string
	Status      string
	Raw         []byte
	At          time.Time
}

// New validates config and returns a Client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("ap_http: FACTURA_PEPPOL_AP_URL required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("ap_http: invalid BaseURL")
	}
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		selfID:      cfg.SelfID,
		callbackURL: cfg.CallbackURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the adapter id (not a commercial brand).
func (c *Client) Name() string { return "http" }

// Ping hits GET /v1/health.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/health", nil)
	if err != nil {
		return err
	}
	c.auth(req)
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ap_http: ping: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("ap_http: ping status %d", res.StatusCode)
	}
	return nil
}

// Send posts the SBD XML. Caller must supply a complete SBD body via
// EncodeSBD helper or pass pre-built XML through Message.Payload as the
// *invoice* and let Send wrap — here we expect the parent to pass invoice
// payload and we build a minimal JSON+XML hybrid for the contract.
//
// Actually: parent passes BisMessage and factory encodes ToXML then calls
// SendXML. Keep Send accepting Message and building XML here would duplicate
// envelope logic. Factory bridge encodes first — see SendSBD.
func (c *Client) Send(ctx context.Context, m Message) (Receipt, error) {
	// Build a minimal envelope JSON for the HTTP contract when only payload
	// fields are set; preferred path is SendSBD with full XML.
	body, err := buildSBDXML(m)
	if err != nil {
		return Receipt{}, err
	}
	return c.SendSBD(ctx, m.APMessageID, body)
}

// SendSBD posts raw SBD XML with idempotency key (single attempt).
// Retries live in peppol.SendWithRetry so Fake and HTTP share one policy.
func (c *Client) SendSBD(ctx context.Context, apMessageID string, sbdXML []byte) (Receipt, error) {
	if apMessageID == "" {
		apMessageID = fmt.Sprintf("ap-%d", time.Now().UnixNano())
	}
	return c.sendOnce(ctx, apMessageID, sbdXML)
}

func (c *Client) sendOnce(ctx context.Context, apMessageID string, sbdXML []byte) (Receipt, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(sbdXML))
	if err != nil {
		return Receipt{}, err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("X-Idempotency-Key", apMessageID)
	res, err := c.http.Do(req)
	if err != nil {
		return Receipt{}, &retryableError{err: err}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 500 {
		return Receipt{}, &retryableError{err: fmt.Errorf("ap_http: status %d", res.StatusCode)}
	}
	if res.StatusCode >= 400 {
		return Receipt{}, fmt.Errorf("ap_http: status %d: %s", res.StatusCode, truncate(raw, 200))
	}
	var parsed struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(raw, &parsed)
	id := parsed.ID
	if id == "" {
		id = apMessageID
	}
	status := parsed.Status
	if status == "" {
		status = "delivered"
	}
	return Receipt{
		APMessageID: id,
		Status:      status,
		Raw:         raw,
		At:          time.Now().UTC(),
	}, nil
}

// Poll fetches inbound messages since the given time.
func (c *Client) Poll(ctx context.Context, since time.Time) ([]Message, error) {
	u := c.baseURL + "/v1/messages?since=" + url.QueryEscape(since.UTC().Format(time.RFC3339))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ap_http: poll: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("ap_http: poll status %d", res.StatusCode)
	}
	var parsed struct {
		Messages []struct {
			ID            string `json:"id"`
			SBDXMLBase64  string `json:"sbd_xml_base64"`
			SenderID      string `json:"sender_id"`
			ReceiverID    string `json:"receiver_id"`
			PayloadBase64 string `json:"payload_base64"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ap_http: poll json: %w", err)
	}
	out := make([]Message, 0, len(parsed.Messages))
	for _, m := range parsed.Messages {
		msg := Message{APMessageID: m.ID, SenderID: m.SenderID, ReceiverID: m.ReceiverID, CreationTime: time.Now().UTC()}
		if m.PayloadBase64 != "" {
			b, err := base64.StdEncoding.DecodeString(m.PayloadBase64)
			if err != nil {
				return nil, err
			}
			msg.Payload = b
		} else if m.SBDXMLBase64 != "" {
			sbd, err := base64.StdEncoding.DecodeString(m.SBDXMLBase64)
			if err != nil {
				return nil, err
			}
			// Leave payload as full SBD — parent unwraps via peppol.FromXML.
			msg.Payload = sbd
		}
		out = append(out, msg)
	}
	return out, nil
}

// Register posts a participant to the AP directory.
func (c *Client) Register(ctx context.Context, p Participant) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"peppol_id":    p.ID,
		"service_type": p.ServiceType,
		"name":         p.Name,
		"callback_url": c.callbackURL,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/participants", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("ap_http: register status %d: %s", res.StatusCode, truncate(raw, 200))
	}
	var parsed struct {
		Endpoint string `json:"endpoint"`
	}
	_ = json.Unmarshal(raw, &parsed)
	return parsed.Endpoint, nil
}

func (c *Client) auth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

type retryableError struct{ err error }

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var r *retryableError
	return err != nil && (asRetryable(err, &r) || strings.Contains(strings.ToLower(err.Error()), "timeout"))
}

func asRetryable(err error, target **retryableError) bool {
	for err != nil {
		if e, ok := err.(*retryableError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// buildSBDXML produces a minimal SBD for the HTTP adapter when the parent
// only supplies Message fields. Prefer peppol.BisMessage.ToXML at the engine
// layer; this is a fallback for direct adapter use.
func buildSBDXML(m Message) ([]byte, error) {
	if len(m.Payload) == 0 {
		return nil, fmt.Errorf("ap_http: empty payload")
	}
	// Delegate shape: wrap as Factura-dialect SBD using the same tags the
	// engine uses — duplicated minimally to keep ap_http free of peppol import.
	created := m.CreationTime
	if created.IsZero() {
		created = time.Now().UTC()
	}
	msgID := m.APMessageID
	if msgID == "" {
		msgID = fmt.Sprintf("ap-%d", created.UnixNano())
	}
	proto := m.CollaborationProtocol
	if proto == "" {
		proto = "urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	}
	proc := m.Process
	if proc == "" {
		proc = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
	}
	b64 := base64.StdEncoding.EncodeToString(m.Payload)
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<StandardBusinessDocument xmlns="urn:fdc:peppol.eu:2017:poacc:billing:3.0:sbd">
  <StandardBusinessDocumentHeader>
    <HeaderVersion>1.0</HeaderVersion>
    <Sender><Identifier Authority="iso6523-actorid-upis">%s</Identifier></Sender>
    <Receiver><Identifier Authority="iso6523-actorid-upis">%s</Identifier></Receiver>
    <DocumentIdentification>
      <Standard>%s</Standard>
      <TypeVersion>3.0</TypeVersion>
      <InstanceIdentifier>%s</InstanceIdentifier>
      <Type>Invoice</Type>
      <CreationDateAndTime>%s</CreationDateAndTime>
    </DocumentIdentification>
    <BusinessScope>
      <Scope><Type>PROCESSID</Type><InstanceIdentifier>%s</InstanceIdentifier><Identifier>%s</Identifier></Scope>
    </BusinessScope>
  </StandardBusinessDocumentHeader>
  <Payload mimeType="text/xml" contentType="application/xml" encoding="base64">%s</Payload>
  <Control>
    <Sender>%s</Sender>
    <Receiver>%s</Receiver>
    <CreationTime>%s</CreationTime>
    <APMessageID>%s</APMessageID>
  </Control>
</StandardBusinessDocument>
`, xmlEscape(m.SenderID), xmlEscape(m.ReceiverID), xmlEscape(proto), xmlEscape(msgID),
		created.UTC().Format(time.RFC3339), xmlEscape(proc), xmlEscape(proc), b64,
		xmlEscape(m.SenderID), xmlEscape(m.ReceiverID), created.UTC().Format(time.RFC3339), xmlEscape(msgID))
	return []byte(xml), nil
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
