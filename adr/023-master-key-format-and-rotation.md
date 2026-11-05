# ADR 023: Master key format and key-version rotation path

## Status

Accepted (2026-11-05). Extends ADR 018 (Coolify enrichment and access), which
placed Coolify tokens and TOTP seeds behind the master-key encryption boundary
that notification channels already used. Implemented by P1-04 of the v1.0
plan (findings SEC-4, SEC-5, SEC-11, SEC-17).

## Context

`internal/auth/secrets.go` encrypts UI-entered secrets with AES-256-GCM under
one master key from `BINNACLE_MASTER_KEY` or `BINNACLE_MASTER_KEY_FILE`. The
v1.0 audit found three weaknesses around that boundary:

- **Any 32-character string was key material** (SEC-11). A passphrase, a
  truncated hex value, or the literal placeholder from an example file became
  the AES key as raw bytes. Hex and base64 were tried only after that, so the
  same bytes could be read two ways depending on length, and a typo was never
  reported.
- **No rotation path.** `encrypted_secrets.key_version` has existed since
  migration 004, but the store compared it with a constant and could hold
  only one key. Rotating the master key meant losing every stored secret.
- **Secrets outlived the target they were entered for** (SEC-4, SEC-5,
  SEC-17). A stored Coolify token was reused for a connection test against any
  URL; a webhook URL or SMTP host could be changed while the bearer token,
  signing secret or password stayed attached; deleting a channel left its
  secret row in place.

## Decision

### 1. The master key is 64 hex characters or base64 of 32 bytes

`NewSecretStore` accepts exactly two encodings of a 32-byte key: 64
hexadecimal characters, or base64 in the standard or URL alphabet with or
without padding. Anything else fails with
`master encryption key must be 64 hexadecimal characters or the base64
encoding of exactly 32 bytes`, and `cmd/binnacle` exits at startup. Whitespace
is not trimmed beyond the trailing newline the file loader already removes.

Hex and base64 encodings of the same bytes produce the same key, so a
deployment that used a raw 32-character value re-encodes those bytes as hex
and keeps every stored secret. `docs/operations/upgrade.md` carries the
command.

### 2. The store is a keyring indexed by `key_version`

`SecretStore` holds `map[int]cipher.AEAD` and a current version. `Put` seals
with the current key and stamps its version; `Get` reads the row's version and
opens with that entry, failing with a message that names the missing version
when the keyring does not hold it. Today the keyring holds one entry, version
1, and `KeyVersion()` reports it. No schema change is needed: the column, its
`NOT NULL` constraint and the `Status` surface already exist.

### 3. Rotation, when it is built, works like this

A later task adds `BINNACLE_MASTER_KEY_PREVIOUS[_FILE]` loaded as version
`current-1`, bumps `SecretKeyVersion`, and on startup rewraps every row whose
version is below current inside one transaction per row, then logs how many
remain. Reads keep working throughout because `Get` already selects by row
version. Once no row references the previous version the operator removes the
previous key. This ADR fixes the data model so that task is additive; it does
not implement the rewrap.

### 4. Secrets are bound to the target they were entered for

- The Coolify token is bound to the origin (scheme, host, port) of the URL it
  was saved with. `Configure` without a token succeeds only on the same
  origin; another origin returns `ErrTokenBoundToOrigin`. `Test` uses only the
  submitted token and returns `ErrTokenRequired` without one; with
  environment-authoritative configuration it tests the environment pair.
- A notification channel patch that changes the webhook URL clears the bearer
  token and signing secret; one that changes the SMTP host clears the
  password; secrets entered in the same patch are kept. The same `apply`
  function produces the candidate the API validates and the value the
  repository stores.
- Deleting a channel deletes its `encrypted_secrets` row in the same
  transaction as the soft delete.

## Consequences

- Operators with a raw 32-character key must re-encode it once before
  upgrading; the container tells them so instead of silently using a weaker
  key. New installs generate `openssl rand -hex 32`.
- Secrets never travel to a host their owner did not enter them for. The cost
  is re-typing a token or password when a target moves, which the forms now
  say up front.
- Key rotation becomes a bounded, additive change rather than a data
  migration.

## Alternatives

- **Derive a key from any string with a KDF.** Hides typos and short inputs
  behind strong-looking output and still gives no rotation path. Rejected.
- **Bind the Coolify token to the full URL.** Path edits such as adding
  `/api/v1` would require the token again for no security gain. Origin is the
  boundary a bearer token actually crosses.
- **Refuse URL or host changes while a secret is stored.** Forces delete and
  recreate, which loses delivery history. Clearing the dependent secrets keeps
  the channel and its history.
