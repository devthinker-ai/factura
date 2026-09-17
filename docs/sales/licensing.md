# Factura — plans & licensing (sales / fulfillment)

Buyer-facing caps + how ops mints keys after checkout. Technical source of truth: [`docs/licensing.md`](../licensing.md). Indicative prices — validate before quoting (GTM §5.5).

## Plans

| Plan | Indicative | Companies | API keys | Peppol | Clients | Fit |
|------|------------|-----------|----------|--------|---------|-----|
| **Solo** | ~€150/yr | 1 | 2 | — | — | One company, receive + issue DE |
| **Pro** | ~€40–60/mo | 10 | 20 | ✓ | 10 | Multi-company, Peppol, API keys |
| **Kanzlei** | ~€150–400/mo | 50 | 50 | ✓ | 50 | Tax-advisor scale / design partners |
| **Embed** | per-1k ops | — | metered | ✓ | — | ISV volume (Engine B) |

**Talk track:** licenses are offline RS256 JWTs — the binary never phones home. Lemon Squeezy is checkout / merchant-of-record only.

**Fail-open:** missing, invalid, or expired keys degrade to **Solo** caps and log a warning. Air-gapped boxes keep processing.

## Customer activation

1. Customer receives the JWT (email / LS download).
2. Paste in console (**Settings → License**) or set `FACTURA_LICENSE_KEY`.
3. Caps apply live — no restart.
4. Renewal = paste the new key. Mistyped key: `factura license remove` or DELETE `/license`.

## Mint a key (owner machine)

Private key at `~/.factura-license-priv.pem` — **never in the repo**. Public key is embedded in the binary.

```bash
factura license mint \
  --plan pro \
  --subject customer@example.com \
  --expires 2027-12-31 \
  > key.txt
```

`--plan` is `solo` | `pro` | `kanzlei`. Optional overrides: `--companies N`, `--api-keys N`, `--clients N`, `--key /path/to/priv.pem`.

Verify before sending:

```bash
factura license check --license "$(cat key.txt)"   # exit 0 licensed, 4 unlicensed
```

## Fulfillment checklist

1. Customer buys Solo / Pro / Kanzlei on Lemon Squeezy.
2. Note plan + subject email + paid-through date (webhook or CSV).
3. Mint JWT on the owner machine (command above).
4. Hand the key to the customer.
5. On renewal, mint a new key with the new `--expires` and they paste it again.
