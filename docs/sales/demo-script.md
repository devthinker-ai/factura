# Factura — 30-minute demo script

**Goal:** Leave them saying: “That report is exactly what our Steuerberater asked for — and the data stays with us.”

## Prep

- Running instance: `factura serve --peppol-fake` (or Docker)
- One **valid** golden XRechnung + one **intentionally invalid** file
- Optional Pro license key to show Peppol / multi-company caps
- Mandate slide (2025 receive / 2027 issue)
- Compliance checklist PDF as leave-behind

## Script

| Min | Move | Say / show |
|-----|------|------------|
| 0–3 | Mandate frame | Receive already law; issue 2027 cliff. Ask where invoices land today. |
| 3–8 | Ingest real file | Drop/POST supplier file. Show valid/invalid + human DE text + rule ID. |
| 8–12 | Archive proof | Detail: original download, GOBL, GoBD evidence / `factura verify`. |
| 12–18 | Issue path | Create standard or credit note; Leitweg-ID if B2G; download XR + ZUGFeRD. |
| 18–23 | Peppol | Send/receive ledger (fake AP OK). Stress AP is swappable; receipts archived. |
| 23–27 | Ops / embed | Match their stack: docker, mail-relay curl, or API key + SDK. |
| 27–30 | Close | Checklist + next step: Solo, design partner, or embed workshop. |

## Do not demo

- Bookkeeping screens you do not have
- Vague “AI will fix VAT”
- Over-claiming FR/BE deep validation
- Hard-printing disputed small-biz exemption end dates

## Success signals

- They ask about self-host / data residency
- They want to bring their Steuerberater to the next call
- They ask for API docs / sample GOBL
- They name a live client or vendor integration date
