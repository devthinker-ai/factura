# Factura — pitch deck outline (7 slides)

Use with a live sandbox. Timing: ~20 minutes talk + ~10 minutes demo/Q&A.

---

### Slide 1 — Hook

**Title:** Your customers already send e-invoices. Can you prove they are valid?

- DE receive mandate: **in force since 2025-01-01**
- DE issue mandate: **2027-01-01** for most B2B
- Anxiety is real; incumbent answer is often “buy our suite”

**Ask:** Where do supplier e-invoices land today — and who checks validity?

---

### Slide 2 — The problem

| Option | What you get | What it costs |
|--------|--------------|---------------|
| Accounting suite bolt-on | E-invoice as seat tax | Lock-in, cloud archive, per-user fees |
| Peppol-only transport | Delivery | Weak validity + archive story |
| DIY / open the XML | Hope | No citeable rule findings |

---

### Slide 3 — The answer

**Factura is the compliance appliance.**

One static binary (or Docker). Feed invoices (API, mail, folder, Peppol). Get plain-language valid/invalid + why. GoBD-style archive. Issue XRechnung + ZUGFeRD. Offline license. Your server. Your data.

**Line to memorize:**

> You don’t buy Factura to run your accounting. You buy it so the e-invoice part stops being a tax on your ERP.

---

### Slide 4 — One job, done right

**We do:** parse · validate · archive · issue · Peppol SP  

**We never do:** ledger · bank · payroll · CRM · become an Access Point

Scope discipline is the moat.

---

### Slide 5 — How it lands

| Path | First win |
|------|-----------|
| `docker run` / one binary | Validating inbox tonight |
| Mail relay / watch folder | Zero UI change for staff |
| HTTP API + SDKs | E-invoicing inside their product |
| Console + roles + MFA | Shared finance / Kanzlei workspace |

---

### Slide 6 — Commercial (indicative)

| Plan | Indicative | Caps | Fit |
|------|------------|------|-----|
| Solo | ~€150/yr | 1 company, 2 API keys | Receive + issue DE |
| Pro | ~€40–60/mo | 10 companies, 20 keys, Peppol | Multi-company / API |
| Kanzlei | ~€150–400/mo | 50 / 50 / Peppol | Advisor scale / design partners |
| Embed | per-1k ops | Metered | ISV volume |

Checkout via Lemon Squeezy; license is an offline JWT — **no phone-home**. Full caps + mint flow: [licensing.md](./licensing.md).

---

### Slide 7 — Close

**Be compliant before the 2027 cliff — without renting your archive.**

Next step options:

1. 30-minute live sandbox (their real file)
2. Solo license for one company
3. Steuerberater design-partner pilot
4. Embed technical workshop (2-week path)

Leave-behind: compliance checklist PDF (lead magnet).
