package peppol

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// SBD XML — Factura's stable BIS 3.0 envelope dialect.
// Pin: Peppol BIS Billing 3.0 SBD wrapping EN 16931 XML payload.

// Wire format (stdlib encoding/xml). Payload is base64 so round-trip is
// byte-for-byte lossless for arbitrary invoice XML.
type sbdDoc struct {
	XMLName xml.Name `xml:"urn:fdc:peppol.eu:2017:poacc:billing:3.0:sbd StandardBusinessDocument"`
	Header  sbdHeader `xml:"StandardBusinessDocumentHeader"`
	Payload sbdPayload `xml:"Payload"`
	Control sbdControl `xml:"Control"`
}

type sbdHeader struct {
	HeaderVersion         string    `xml:"HeaderVersion"`
	Sender                sbdParty  `xml:"Sender"`
	Receiver              sbdParty  `xml:"Receiver"`
	DocumentIdentification sbdDocID `xml:"DocumentIdentification"`
	BusinessScope         sbdScope  `xml:"BusinessScope"`
}

type sbdParty struct {
	Identifier sbdIdentifier `xml:"Identifier"`
	Contact    string        `xml:"ContactInformation>Contact,omitempty"`
}

type sbdIdentifier struct {
	Authority string `xml:"Authority,attr,omitempty"`
	Value     string `xml:",chardata"`
}

type sbdDocID struct {
	Standard         string `xml:"Standard"`
	TypeVersion      string `xml:"TypeVersion"`
	InstanceIdentifier string `xml:"InstanceIdentifier"`
	Type             string `xml:"Type"`
	CreationDateAndTime string `xml:"CreationDateAndTime"`
}

type sbdScope struct {
	Scope []sbdScopeItem `xml:"Scope"`
}

type sbdScopeItem struct {
	Type      string `xml:"Type"`
	InstanceID string `xml:"InstanceIdentifier"`
	ID        string `xml:"Identifier"`
}

type sbdPayload struct {
	MimeType    string `xml:"mimeType,attr"`
	ContentType string `xml:"contentType,attr"`
	Encoding    string `xml:"encoding,attr"`
	Data        string `xml:",chardata"`
}

type sbdControl struct {
	Sender       string `xml:"Sender,omitempty"`
	Receiver     string `xml:"Receiver,omitempty"`
	ServicePoint string `xml:"ServicePoint,omitempty"`
	CreationTime string `xml:"CreationTime,omitempty"`
	APMessageID  string `xml:"APMessageID,omitempty"`
	Status       string `xml:"Status,omitempty"`
}

// ToXML serializes the BIS 3.0 SBD envelope. Payload bytes are embedded
// base64 (not re-parsed) so FromXML restores them exactly.
func (m BisMessage) ToXML() ([]byte, error) {
	if len(m.Payload) == 0 {
		return nil, fmt.Errorf("peppol: empty payload")
	}
	proto := m.CollaborationProtocol
	if proto == "" {
		proto = CollaborationProtocol
	}
	proc := m.Process
	if proc == "" {
		proc = ProcessURI
	}
	mime := m.PayloadMime
	if mime == "" {
		mime = PayloadMimeXML
	}
	created := m.Control.CreationTime
	if created.IsZero() {
		created = time.Now().UTC()
	}
	msgID := m.Control.APMessageID
	if msgID == "" {
		msgID = fmt.Sprintf("factura-%d", created.UnixNano())
	}

	doc := sbdDoc{
		Header: sbdHeader{
			HeaderVersion: "1.0",
			Sender: sbdParty{
				Identifier: sbdIdentifier{Authority: "iso6523-actorid-upis", Value: m.Sender.Identifier},
				Contact:    m.Sender.ServicePointURL,
			},
			Receiver: sbdParty{
				Identifier: sbdIdentifier{Authority: "iso6523-actorid-upis", Value: m.Receiver.Identifier},
				Contact:    m.Receiver.ServicePointURL,
			},
			DocumentIdentification: sbdDocID{
				Standard:           proto,
				TypeVersion:        "3.0",
				InstanceIdentifier: msgID,
				Type:               "Invoice",
				CreationDateAndTime: created.UTC().Format(time.RFC3339),
			},
			BusinessScope: sbdScope{
				Scope: []sbdScopeItem{
					{Type: "DOCUMENTID", InstanceID: msgID, ID: proto},
					{Type: "PROCESSID", InstanceID: proc, ID: proc},
				},
			},
		},
		Payload: sbdPayload{
			MimeType:    mime,
			ContentType: PayloadContentType,
			Encoding:    "base64",
			Data:        base64.StdEncoding.EncodeToString(m.Payload),
		},
		Control: sbdControl{
			Sender:       firstNonEmpty(m.Control.Sender, m.Sender.Identifier),
			Receiver:     firstNonEmpty(m.Control.Receiver, m.Receiver.Identifier),
			ServicePoint: firstNonEmpty(m.Control.ServicePoint, m.Sender.ServicePointURL),
			CreationTime: created.UTC().Format(time.RFC3339),
			APMessageID:  msgID,
			Status:       m.Control.Status,
		},
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("peppol: encode sbd: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FromXML parses a BIS 3.0 SBD envelope produced by ToXML (or a compatible
// Factura-dialect SBD). Payload is decoded losslessly from base64.
func FromXML(data []byte) (BisMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return BisMessage{}, fmt.Errorf("peppol: empty sbd")
	}
	var doc sbdDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return BisMessage{}, fmt.Errorf("peppol: parse sbd: %w", err)
	}
	if doc.Header.DocumentIdentification.Standard == "" && doc.Payload.Data == "" {
		return BisMessage{}, fmt.Errorf("peppol: not a BIS 3.0 SBD envelope")
	}

	payload, err := decodePayload(doc.Payload)
	if err != nil {
		return BisMessage{}, err
	}

	proto := doc.Header.DocumentIdentification.Standard
	if proto == "" {
		proto = CollaborationProtocol
	}
	proc := ProcessURI
	for _, sc := range doc.Header.BusinessScope.Scope {
		if strings.EqualFold(sc.Type, "PROCESSID") {
			if sc.InstanceID != "" {
				proc = sc.InstanceID
			} else if sc.ID != "" {
				proc = sc.ID
			}
		}
	}

	created, _ := time.Parse(time.RFC3339, doc.Control.CreationTime)
	if created.IsZero() {
		created, _ = time.Parse(time.RFC3339, doc.Header.DocumentIdentification.CreationDateAndTime)
	}

	msgID := doc.Control.APMessageID
	if msgID == "" {
		msgID = doc.Header.DocumentIdentification.InstanceIdentifier
	}

	mime := doc.Payload.MimeType
	if mime == "" {
		mime = PayloadMimeXML
	}

	return BisMessage{
		CollaborationProtocol: proto,
		Process:               proc,
		Sender: PartyID{
			Identifier:      strings.TrimSpace(doc.Header.Sender.Identifier.Value),
			ServicePointURL: strings.TrimSpace(doc.Header.Sender.Contact),
		},
		Receiver: PartyID{
			Identifier:      strings.TrimSpace(doc.Header.Receiver.Identifier.Value),
			ServicePointURL: strings.TrimSpace(doc.Header.Receiver.Contact),
		},
		Payload:     payload,
		PayloadMime: mime,
		Control: Control{
			Sender:       firstNonEmpty(doc.Control.Sender, strings.TrimSpace(doc.Header.Sender.Identifier.Value)),
			Receiver:     firstNonEmpty(doc.Control.Receiver, strings.TrimSpace(doc.Header.Receiver.Identifier.Value)),
			ServicePoint: doc.Control.ServicePoint,
			CreationTime: created,
			APMessageID:  msgID,
			Status:       doc.Control.Status,
		},
	}, nil
}

func decodePayload(p sbdPayload) ([]byte, error) {
	raw := strings.TrimSpace(p.Data)
	if raw == "" {
		return nil, fmt.Errorf("peppol: empty payload")
	}
	enc := strings.ToLower(strings.TrimSpace(p.Encoding))
	if enc == "" || enc == "base64" {
		out, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			// Some APs may embed raw XML without encoding attr.
			if strings.HasPrefix(raw, "<") {
				return []byte(raw), nil
			}
			return nil, fmt.Errorf("peppol: payload base64: %w", err)
		}
		return out, nil
	}
	return []byte(raw), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ExternalIDFromMessage picks a stable external_id for Ingest from the SBD.
func ExternalIDFromMessage(m BisMessage) string {
	if m.Control.APMessageID != "" {
		return "peppol:" + m.Control.APMessageID
	}
	return fmt.Sprintf("peppol:%s-%s-%d", m.Sender.Identifier, m.Receiver.Identifier, m.Control.CreationTime.UnixNano())
}
