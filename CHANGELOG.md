# Changelog

## [0.9.2] — 2026-09-17 — Skip Docker smoke in default tests

### Fixed
- `TestDockerSmoke` is opt-in (`FACTURA_DOCKER_SMOKE=1`). On GitHub Actions the
  daemon is present, so `go test ./...` was building the full image and failing
  the release job (~5 min). Release still builds/pushes the image via Buildx.

## [0.9.1] — 2026-09-17 — Release pipeline fix

### Fixed
- Release workflow no longer runs `npm ci` / SPA build on the runner (that step
  failed on `v0.9.0`). Binaries and the GHCR image use the committed
  `pkg/web/dist` embed, matching the Dockerfile. Refresh with `make frontend`
  before tagging.

## [0.9.0] — 2026-09-17 — Phases 9–11 + Docker try-path

### Added
- **Phase 11 — branding:** Per-company PDF branding (`companies.branding_json`):
  logo (PNG/JPEG → server JPEG), accent color, header/footer text. Rewritten
  ZUGFeRD page renderer with line-item table, totals, multi-page support, and
  optional logo XObject (`GenerateWith`). Console Settings → Branding with live
  PDF preview; `company_id` on generate. `GET /companies/{id}` returns full
  branding (incl. logo); list omits `logo_b64`.
- **Phase 10 — optional EN 16931 fields:** New-invoice form exposes project /
  contract / purchase / sales / tender refs (BT-11/12/13/14/17), buyer
  accounting (BT-19), delivery date (BT-72), invoice period (BT-73/74),
  seller/buyer identifiers (BT-29/46), legal registration (BT-30), remittance
  (BT-83), free-text payment terms (BT-20), invoice-level discounts (BT-107),
  and a country picker (BT-40/55, label-only — DE regime unchanged). Collapsible
  Referenzen / Zeitraum / Nachlässe; `previewTotals` applies discounts.
- **Phase 9 — issuance completeness:** New-invoice seller contact + phone
  (BT-41/42), buyer email (BT-49), payment method Überweisung/Lastschrift
  (BG-19), optional BIC / account holder (BT-85/86), already-paid (BT-113),
  Kleinunternehmer / Steuernummer (BT-32). Issuance pre-pass
  `FACTURA-ATTACHMENT-MIME` (BT-125 allow-list) + matching client-side rejection.
- **Packaging:** GitHub Actions release on `v*` tags (cross-compiled binaries +
  GHCR `ghcr.io/devthinker-ai/factura`); `docker-compose.yml`; Dockerfile
  defaults `FACTURA_DB=/data/factura.db`. README 5-minute quickstart is a
  one-liner `docker run`. Console Users nav visible in open mode so the first
  admin can bootstrap from the UI.

### Fixed
- Console `partyToGobl` emitted seller email under the wrong GOBL key
  (`address` instead of `addr`), so BT-43 was silently dropped and
  console-issued invoices failed KoSIT self-check (`BR-DE-5/6/7`,
  `PEPPOL-EN16931-R010`). Now emits `addr`/`num`, `inboxes`, and `people`.

### Invariants
- XRechnung CII stays byte-identical with or without branding (PDF-layer only).
- `PATCH /companies/{id}` with `{}` still sets default (frontend compat).

### Known limits
- BT-33 (additional legal info) left unwired — see AGENTS.md.

### Deps
Zero new dependencies. Engine packages frozen except additive pre-pass /
`pkg/generate` renderer.

## [0.8.0] — 2026-09-11 — Phase 8: B2G / issuance readiness (credit notes, attachments, GoBD)

### Added
- Document types for issue: credit note (381), corrected (384), self-billed (389)
  via `model.NewCreditNote` / `NewCorrected` / `NewSelfBilled` + `TypeCode()`.
- Embedded attachments (BT-125 data-URI, max 200) and Leitweg-ID / buyer
  reference (BT-10 → `ram:BuyerReference`).
- Issuance pre-pass: `FACTURA-PRECEDING-REQUIRED`, `FACTURA-PRECEDING-DATE`,
  `FACTURA-ATTACHMENT-LIMIT`, `FACTURA-ATTACHMENT-EMPTY`, `FACTURA-LEITWEG`.
- GoBD append-only hash chain (`chain_links`); every ingest row is sealed.
  `factura verify`, `GET /audit`, `GET /invoices/{id}/evidence`.
- Additive `invoice_type` on archive rows + CSV export.
- Console: document-type select, preceding ref, attachments, Leitweg-ID;
  detail page type chip + GoBD evidence block.

### Status codes
Unchanged (invariant #6). Generate 422 bodies may include `detail` with the
pre-pass human text. CLI exit `1` = broken chain (`factura verify`).

### Deps
Zero new dependencies.

## [0.7.0] — 2026-09-11 — Phase 7: Console auth (users + roles + TOTP MFA)

### Added
- Console users with roles `admin` / `editor` / `viewer`, argon2id passwords,
  HS256 session JWTs (24 h), optional TOTP MFA + 10 recovery codes.
- Public `POST /login`, `POST /login/mfa`, `GET /auth-status`; authed
  `GET /me`, user CRUD (admin), self-service `POST/DELETE /2fa*`.
- First-user bootstrap: public `POST /users` on empty DB (must be `admin`),
  or `factura user add|list|remove|password|role`.
- Console: `/app/login` (DE/EN), Settings → Security (2FA), Settings → Users;
  role-aware nav (viewer hides New invoice / Peppol send / settings writes).
- `GET /health` additive `"auth": "open"|"token"|"users"`.

### Status codes (additive)
Role denial is **`401` `{"error":"forbidden"}`** — `403` remains plan/quota
only. Login/MFA failures use distinct bodies (`invalid credentials`,
`invalid code`, `mfa token expired or already used`); open/token modes stay
byte-identical to 0.6.0.

### Deps
`github.com/pquerna/otp`, `github.com/skip2/go-qrcode`; `golang.org/x/crypto`
promoted to a direct require (argon2id).

## [0.6.0] — 2026-09-10 — Phase 6: Licensing + SaaS embedding

Revenue mechanism: offline RS256 licenses (Solo / Pro / Kanzlei, fail-open to
Solo) and per-product API keys with deterministic invoice-op metering.
Companies registry, Settings → License console, `sdk/go` + `sdk/js`,
`docs/EMBED.md`. Engine (parse/validate/generate/peppol) frozen; Peppol gated
behind Pro. New dep: `github.com/golang-jwt/jwt/v5` only.

## [0.5.0] — 2026-09-10 — Phase 5: Console UX, light/dark theme, DE/EN i18n

Frontend-only polish (backend / API / Peppol frozen). Hand-rolled i18n (zero
new deps) — `locales/de.json` + `en.json` + `useT()`; prefer over
`react-i18next` for bundle size.

### Added
- **Light / dark / system theme** — sidebar footer toggle (Sun/Moon/Monitor),
  persisted `localStorage` `factura:theme`, no-flash inline boot in
  `index.html`, live `prefers-color-scheme` while in system mode.
- **DE/EN UI chrome** — language switcher (DE|EN) next to theme; default
  `de` when `navigator.language` is `de-*`, else `en`; persisted
  `factura:lang`. Validation `human` / engine payload text stays **verbatim**
  (never passed through `t()`).
- **UX pass** — empty states with CTAs, first-run onboarding (empty archive +
  Peppol unconfigured), loading skeletons, recoverable ErrorState + Retry,
  keyboard-navigable inbox/Peppol tables, rule-ID copy-to-clipboard, sticky
  report header / back-to-top on long violation lists.

### Changed
- Sidebar footer hosts theme + language controls; all page chrome strings
  go through i18n catalogs.
- Dark-mode `--muted-foreground` contrast bump for WCAG AA body text.

### Status codes
Unchanged (API frozen).

## [0.4.0] — 2026-09-10 — Phase 4: Peppol (SP behind a swappable AP)

### Added
- `pkg/peppol` — BIS Billing 3.0 SBD envelope (stdlib XML), `AccessPoint`
  interface, in-memory **Fake** (all engine tests offline), `Engine` receive/send.
- `pkg/peppol/ap_http` — one HTTP Access Point adapter against a documented
  contract (`// adapter: verify-against-real-AP`); swap via `FACTURA_PEPPOL_AP`.
- Store tables `participants` + `peppol_messages` (GoBD: BIS XML + receipt blobs).
- API (auth group): `POST/GET /peppol/inbound`, `POST /peppol/send`,
  `POST /peppol/send/{id}/retry`, `GET /peppol/outbound|messages`,
  participants + status + ping.
- CLI: `factura peppol send|participants|status|poll`; `serve --peppol-fake`;
  optional poller (`FACTURA_PEPPOL_POLL=1`).
- Console: Peppol ledger, Settings → Peppol, “Send via Peppol” on invoice detail.

### Status codes (additive)
`424` AP rejected (Peppol only) · `502` AP unreachable after retries.
Phase 2/3 `/invoices*` contract unchanged.

## [0.3.0] — 2026-09-09 — Phase 3: Web UI (compliance console)

### Added
- Embedded SPA at `/app/` (`pkg/web`, Vite + React + Tailwind) — Inbox,
  invoice detail (validation report), New invoice (GOBL → XR + ZUGFeRD).
- `GET /` → 302 to `/app/` (HTML meta-refresh fallback).
- `GET /invoices/vendors` — distinct vendor names for the Inbox filter.
- `GET /health` additive `"version"` field (ldflags / `VERSION` file, default
  `dev`).
- Operator token bar in the UI (`localStorage.factura_token`) for
  `FACTURA_TOKEN` mode — no login form.
- `make frontend`, `make smoke`, embed sync into `pkg/web/dist`.

### Changed
- `make build` refreshes the embed when `frontend/` is newer than
  `pkg/web/dist`.

### Status codes
Unchanged (Phase 2 contract). Vendors endpoint: `200` / `401` / `500`.

## [0.2.0] — 2026-09-09 — Phase 2: HTTP API + watch/batch

### Added
- `factura serve` — chi HTTP API (`pkg/api`): ingest, validate dry-run, list,
  get, original, export, generate (+ preview alias).
- Optional bearer auth via `FACTURA_TOKEN`; rate limit when auth is on
  (`FACTURA_RATE_LIMIT`, default 60/min).
- `factura batch <dir>` and `factura watch <dir>` for folder-driven ingest.
- Dockerfile (`FROM scratch`) and `make run`.
- `invoices.source` column (API `X-Factura-Source`, CLI `--source`).

### Changed
- CSV export header gains trailing `source` column (additive).
- Dependency: `github.com/go-chi/chi/v5` (routing only).

### Status codes (API contract — one condition one code)
`200` / `201` / `401` / `404` / `409` / `413` / `415` / `422` / `429` / `500`

## [0.1.0] — 2026-09-09 — Phase 1: CLI engine

Parse / validate / generate / archive (SQLite). XRechnung 3.0.2 + ZUGFeRD.
