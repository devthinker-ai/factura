# Factura — feature list (buyer-facing)

Status reflects shipped product through **Phase 9** (console issuance field completeness). Phase 10 = multi-client Kanzlei white-label (roadmap).

## Receive & validate

| Feature | Buyer value |
|---------|-------------|
| Detect & parse XRechnung (CII/UBL), ZUGFeRD / Factur-X hybrid PDF | Handles what suppliers actually send |
| EN 16931 + XRechnung business-rule validation | Know if a document is legally sound before it hits the books |
| Human-readable findings in **DE and EN**, with rule IDs | Steuerberater can quote the finding |
| Invalid invoices still archived (never dropped) | Dispute and audit evidence |
| Dry-run validate (no archive) | Safe pre-check |
| HTTP ingest, folder watch, batch, mail-relay recipe | Zero-ERP-change beachhead |
| Idempotent ingest via external ID (409 on retry) | Safe mail-relay retries |
| Source provenance on archive + CSV export | Trace “where did this file come from?” |

## Issue & generate

| Feature | Buyer value |
|---------|-------------|
| Produce XRechnung 3.0.2 | DE B2B / B2G issuance |
| Produce ZUGFeRD 2.3 / Factur-X hybrid PDF (PDF/A-3) | Human + machine in one file |
| Document types: standard (380), credit note (381), corrective (384), self-billed (389) | Real operations, not demo-only |
| Leitweg-ID (BT-10) + syntax check for B2G | Fewer government routing rejections |
| Embedded attachments (BT-125), payment (BG-19), already-paid, Kleinunternehmer / Steuernummer | Fewer self-check failures |
| Console + API field parity for party contacts | What you type is what validates |

## Archive & audit (GoBD substrate)

| Feature | Buyer value |
|---------|-------------|
| SQLite store: original bytes + GOBL JSON + validation report | Searchable compliance memory |
| Append-only hash chain over every archived row | Tamper-evident retention story |
| `factura verify` + API evidence / audit endpoints | Prove integrity on demand |
| Search, filter, vendor list, CSV export | Handoff to advisor / internal controls |

**Positioning note:** Factura is the archive *substrate*. The customer remains the Aufbewahrungspflichtiger. We do not replace legal advice or a canonical DATEV archive policy.

## Peppol (Service Provider)

| Feature | Buyer value |
|---------|-------------|
| BIS Billing 3.0 SBD envelope | Network-ready documents |
| Send + receive behind **one swappable Access Point** | Connectivity without owning an AP |
| Delivery receipts archived | GoBD evidence of transport |
| Fake AP for offline demos / CI | Demo without sockets |

## Platform, security, licensing, embed

| Feature | Buyer value |
|---------|-------------|
| Single static Go binary / Docker (`CGO_ENABLED=0`) | Simple ops; air-gap capable |
| Embedded compliance console (DE/EN, light/dark) | Daily driver for finance |
| Users + roles (admin / editor / viewer) + TOTP MFA | Real office access control |
| Offline RS256 license JWT (Solo / Pro / Kanzlei) | No phone-home to vendor cloud |
| API keys + RPM + monthly invoice-op metering | Product / multi-tenant ops |
| Go + JS SDKs + embed contract | ISV integration in weeks |

## Deliberately out of scope

- Bookkeeping, ledger, bank sync, payroll, CRM, payment chasing
- Becoming a Peppol Access Point
- Deep FR/BE validation until a named design partner pays for it

## Roadmap call-out (sell as design partnership)

| Feature | Status |
|---------|--------|
| Multi-client white-label Kanzlei console + advisor export formats | Phase 10 |
