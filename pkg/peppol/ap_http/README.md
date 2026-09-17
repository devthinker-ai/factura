# ap_http — Factura Peppol Access Point adapter

`// adapter: verify-against-real-AP`

This package is the **only** place that knows how to talk to a Peppol Access
Point. The engine (`pkg/peppol`) speaks through the `AccessPoint` interface;
swap carriers by changing `FACTURA_PEPPOL_AP` (and URL/key).

## Why not a named commercial AP?

Phase 4 ships against a **documented HTTP contract** (below) so the engine is
testable offline with the Fake and deployable once a design partner names
Storecove / GLC / XRoad / a German AP. Pinning a vendor before that would
bake auth quirks into the wrong layer.

## Contract

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/v1/health` | Ping / test connection |
| `POST` | `/v1/messages` | Body = BIS 3.0 SBD XML; `X-Idempotency-Key`; `Authorization: Bearer` |
| `GET` | `/v1/messages?since=` | Poll inbound; JSON with `sbd_xml_base64` or `payload_base64` |
| `POST` | `/v1/participants` | Register participant + callback URL |

Env: `FACTURA_PEPPOL_AP=http`, `FACTURA_PEPPOL_AP_URL`, `FACTURA_PEPPOL_AP_KEY`,
`FACTURA_PEPPOL_SELF_ID`, `FACTURA_PEPPOL_CALLBACK_URL`.

Retries: 3 attempts with 2s / 5s / 15s backoff on network / 5xx only, keyed by
the same idempotency id.
