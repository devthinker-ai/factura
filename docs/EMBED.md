# Embedding Factura — vendor engineer guide

Factura is a self-hosted EU e-invoicing engine. This document is the **embed
contract**: how a DACH product adds ingest / validate / generate against a
Factura box with a per-product API key and deterministic metering.

## Auth

```http
Authorization: Bearer factura_<24 hex chars>
```

- One key per product (or environment). Create via the Factura console
  (Settings → License → API keys) or `POST /api-keys` with the operator token.
- Plaintext is shown **exactly once** at creation. The kill switch
  (`PATCH /api-keys/{id}` `{"enabled":false}`) returns **401** for that key
  immediately — no restart.
- Keys are stored hashed (sha256); list/delete never re-emit a create-only
  `key` field (the `id` is the `factura_…` string the operator already holds).

## Endpoints you need

### Ingest

```bash
curl -sS -X POST "$FACTURA_URL/invoices" \
  -H "Authorization: Bearer $FACTURA_KEY" \
  -H "X-Factura-External-ID: inv-001" \
  --data-binary @invoice.xml
```

### Generate

```bash
curl -sS -X POST "$FACTURA_URL/invoices/generate?format=xrechnung&download=1" \
  -H "Authorization: Bearer $FACTURA_KEY" \
  -H "Content-Type: application/json" \
  -d @invoice.gobl.json -o out.xml
```

### List

```bash
curl -sS "$FACTURA_URL/invoices?status=valid&limit=50" \
  -H "Authorization: Bearer $FACTURA_KEY"
```

### Usage

```bash
curl -sS "$FACTURA_URL/usage" \
  -H "Authorization: Bearer $FACTURA_KEY"
```

Under a key token, `/usage` returns **only that key's row**.

## Status codes (exact)

| Code | When | Body |
|------|------|------|
| **200** | OK (list, usage, health, generate preview) | resource JSON |
| **201** | Created (ingest archived, generate, key create, company) | resource JSON |
| **401** | Missing/invalid/disabled key or operator token | `{"error":"unauthorized"}` |
| **403** | Plan gate or monthly quota | `{"error":"plan_limit","detail":"…"}` or `{"error":"quota_exceeded"}` |
| **404** | Unknown id | `{"error":"not found"}` |
| **409** | Duplicate `external_id` / VAT | `{"error":"…","id":N}` |
| **413** | Body > 10 MB | `{"error":"body too large","limit":10485760}` |
| **415** | Unrecognized invoice/Peppol format (row still archived) | `{"error":"…","detail":"…","id":N}` |
| **422** | Generate / GOBL / license validation failure | `{"error":"…"}` (+ `detail` for license) |
| **424** | Peppol AP rejected | `{"error":"ap rejected","detail":"…"}` |
| **429** | Rate limited (per-key RPM or operator bucket) | `{"error":"rate limited"}` + `Retry-After: 1` |
| **502** | Peppol AP unreachable | `{"error":"ap unreachable"}` |
| **500** | Internal | `{"error":"internal"}` |

**One condition, one code.** 403 is plan/quota only; 429 is rate only.

## Metering (what counts as one invoice operation)

Quote — the unit is deterministic:

> one unit = one **invoice operation** on `POST /invoices` (ingest) or
> `POST /invoices/generate` — counted for **API-key traffic only** (operator
> `FACTURA_TOKEN` and no-auth self-hosted traffic is never metered).
> `POST /peppol/send` counts **exactly 1 when it builds a new document**
> (gobl body) — and that send's **internal** `Ingest` of the artifact
> counts **0**; sending an existing `invoice_id` counts 0.
> Rule of thumb: **meter at the endpoint, never inside `Ingest`**.

Calendar-month rollover: counters reset when the period (`YYYY-MM`) flips.

## Tier caps (self-hosted license)

| Plan | Companies | API keys | Peppol | Clients (Phase 7) |
|------|-----------|----------|--------|-------------------|
| Solo (unlicensed default) | 1 | 2 | — | — |
| Pro | 10 | 20 | ✓ | 10 |
| Kanzlei | 50 | 50 | ✓ | 50 |

Unlicensed / expired / invalid keys **fail open** to Solo caps — the box never
bricks air-gapped.

## Go SDK

```go
import factura "github.com/devthinker-ai/factura/sdk/go"

c := factura.New("http://127.0.0.1:8080", os.Getenv("FACTURA_KEY"))
res, err := c.Ingest(ctx, "inv-001", xmlBytes)
if errors.Is(err, factura.ErrQuota) { /* … */ }
usage, _ := c.Usage(ctx)
```

## JS SDK

Copy [`sdk/js/factura.mjs`](../sdk/js/factura.mjs):

```js
import Factura, { FacturaError } from './factura.mjs'

const f = new Factura(process.env.FACTURA_URL, process.env.FACTURA_KEY)
try {
  const row = await f.ingest('inv-001', xmlBytes)
  console.log(row.id, row.status)
} catch (e) {
  if (e instanceof FacturaError && e.code === 'quota_exceeded') { /* … */ }
}
```

## Key lifecycle

1. Operator creates key → plaintext shown once.
2. Vendor stores it as a secret.
3. Disable (`enabled:false`) for an instant kill; delete for hard revoke.
4. Watch `GET /usage` / console for monthly volume.
