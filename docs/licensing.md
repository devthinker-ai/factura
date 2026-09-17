# Licensing (v1 — deliberately boring)

Factura licenses are **offline RS256 JWTs**. The binary never phones home.
Lemon Squeezy is checkout / merchant-of-record only.

## Flow

1. Customer buys Solo / Pro / Kanzlei on Lemon Squeezy.
2. Fulfillment (webhook or CSV export) tells you plan + subject email + paid-through date.
3. Mint a key on the owner machine (private key at `~/.factura-license-priv.pem`, never in the repo):

```bash
factura license mint \
  --plan pro \
  --subject customer@example.com \
  --expires 2027-12-31 \
  > key.txt
```

4. Hand the key to the customer (email / LS download). They paste it in the
   console (**Settings → License**) or set `FACTURA_LICENSE_KEY`. Caps apply
   live — no restart.
5. Renewal = paste the new key (same path). Mistyped key: `factura license remove`
   or DELETE `/license`.

## Fail-open

Invalid / expired / missing keys degrade to **Solo** caps and log a warning.
An air-gapped invoicing box must keep processing invoices.

## Verify

```bash
factura license check --license "$(cat key.txt)"   # exit 0 licensed, 4 unlicensed
```
