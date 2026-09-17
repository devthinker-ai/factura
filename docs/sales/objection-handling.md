# Factura — objection handling

| Objection | Acknowledge | Reframe | Proof |
|-----------|-------------|---------|-------|
| “We already have DATEV / sevDesk.” | Great — keep it. | Factura is the compliance layer beside it, not a replacement. | API/watch ingest; CSV for the advisor. |
| “Self-hosted is too hard.” | Fair if zero IT. | One binary or one Docker volume. | Live `docker run` in the meeting. |
| “Cloud SaaS is fine.” | Many are. | Ask about 8-year archive control and per-invoice fees at scale. | Offline license + air-gap story. |
| “Are you GoBD certified?” | We are substrate, not Rechtsberatung. | Original + report + hash chain; they remain Aufbewahrungspflichtiger. | `factura verify` / evidence API. |
| “What about Peppol AP fees?” | APs charge per message — true. | Transport separate from compliance software; AP is swappable. | Fake AP demo + adapter architecture. |
| “Why not build?” | Possible with 12+ months and specialists. | Mandate clock + rule maintenance is the real cost. | Shipping validation + issuance types today. |
| “Price?” | Indicative Solo ~€150/yr. | Compare to one rejected invoice or one suite seat-year. | [licensing.md](./licensing.md) caps + design-partner discount. |
| “We need full accounting.” | Understood. | Wrong product — politely walk. Protect scope. | Point them to suite vendors; stay friends. |
| “Multi-client Kanzlei UI?” | On the roadmap (Phase 10). | Design partners shape it; licensing tier already exists. | Roles/MFA + company caps today. |

## Qualification (pursue vs. nurture)

**Pursue when:** receiving now or issuing pilots; wants self-host / data control; has inbox/API/folder or embed need; buyer is Steuerberater, CTO, or finance lead with budget.

**Nurture when:** “wait until 2028” with no advisor pressure; wants full SaaS suite replacement; curious intern with no owner.

## Closing questions

1. Who owns e-invoice compliance for 2027 in your org — by name?
2. Can we validate one real supplier file on a sandbox this week?
3. Is Peppol send required in the first 90 days, or is receive+archive enough to start?
4. For advisors: how many clients would you put on a pilot if multi-client mode matched your export needs?
