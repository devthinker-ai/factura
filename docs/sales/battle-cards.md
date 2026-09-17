# Factura — battle cards

Never trash the incumbent. Reframe: they sell the house; we sell the lock on the e-invoice door.

---

## vs. Accounting suites (lexoffice, sevDesk, DATEV, Sage, …)

**Their frame:** “E-invoicing is included if you move accounting to us.”

**Our frame:** Keep your books. Buy the compliance layer. Flat license, your server, no per-seat tax on invoice volume.

**Landmine question:**  
“If you already love your accounting tool, why switch the whole suite just to open e-invoices correctly?”

**Proof:** API / watch ingest beside existing stack; CSV export for the advisor.

---

## vs. Peppol Access Point / transport SaaS

**Their frame:** “We deliver the document on the network.”

**Our frame:** Delivery without validity is half the job. Factura validates outbound before send, archives receipts as GoBD evidence, and plugs into **one AP you choose**.

**Landmine question:**  
“When the buyer rejects your invoice, does your AP tell you which EN 16931 rule failed — in German?”

**Proof:** Human DE/EN report + Peppol message ledger + swappable AP adapter.

**Honesty:** We are a Service Provider behind an AP — not an AP ourselves. Say it early.

---

## vs. DIY / “our intern will parse the XML”

**Their frame:** “EN 16931 is just XML.”

**Our frame:** Validation is the product. Hundreds of business rules, hybrid PDF, Peppol envelope, hash chain, roles/MFA — already in one box. Mandate clock ≠ science project.

**Landmine question:**  
“Who owns KoSIT-style findings when your customer’s invoice is rejected three weeks before year-end?”

**Proof:** Live invalid fixture → citeable rule IDs in under a minute.

---

## vs. “We’ll wait until 2027”

**Acknowledge:** Issue cliff feels far; receive is **already** law.

**Reframe:** Invoices are arriving structured *now*. Archive and validity debt compounds. Issuance pilots take months.

**Ask:** “How many rejected or unvalidated supplier invoices are sitting in email today?”
