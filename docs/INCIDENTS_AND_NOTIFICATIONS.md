# Incidents and notifications

Incidents automatically group overlapping firing alerts for one affected
target. The first alert opens an incident, later alerts join it, and critical
members promote its severity. An incident resolves after every member alert
resolves. A later alert opens a new incident; acknowledgement, assignment, and
manual resolution are intentionally outside v0.3.

On startup, firing alerts without incident membership are reconciled without
sending historical notifications. Resolved incidents are retained for one
year; completed delivery records are retained for 90 days.

## Channels

Up to 32 channels may be configured under Settings → Notifications. HTTPS webhooks
support an optional bearer token and HMAC-SHA256 signature. SMTP requires
STARTTLS or implicit TLS, one sender, and 1–20 recipients.

Set `BINNACLE_MASTER_KEY_FILE` before creating channels and point it to a
Docker/Coolify secret containing a 32-byte key written as 64 hexadecimal
characters or as base64 (`openssl rand -hex 32` produces one). Raw 32-character
values are rejected at startup with a clear error; see the upgrade guide for
re-encoding an existing key. The legacy `BINNACLE_MASTER_KEY` environment
variable accepts the same formats, but must not be configured at the same
time. If neither is present, monitoring and incidents continue, but channel
creation and delivery report `master_key_missing`. Every stored secret records
the key version that sealed it (ADR 023).

Channel credentials are bound to the target they were entered for. Changing a
webhook URL clears its bearer token and signing secret, and changing the SMTP
host clears the password, unless the same save enters them again; the form
asks for them when a target changes. Deleting a channel deletes its encrypted
secrets immediately, while the channel row stays for delivery history.

New channels receive only later updates or reminders. Open and update events
wait 15 seconds to coalesce simultaneous alerts. If an incident resolves
before its opening delivery is attempted, the pending lifecycle delivery is
cancelled as transient noise. Open incidents receive a reminder every two
hours.

Webhook payloads use schema version 1 and include the event type, delivery and
idempotency identifiers, incident lifecycle state, affected target, alert
counts, and up to 20 member alerts. The stable idempotency key is also sent in
the `Idempotency-Key` header. When an HMAC secret is configured, the request
carries `X-Binnacle-Timestamp` (unix seconds at send time) and
`X-Binnacle-Signature` as `sha256=<hex digest>`, an HMAC-SHA256 over
`<timestamp>.<exact request body>`. Receivers should recompute the digest over
the same string, compare it in constant time, and reject deliveries whose
timestamp is more than five minutes from their own clock; retries are signed
afresh with a new timestamp, so a rejected replay never blocks a real retry.
Receivers written for v0.6.0 and earlier verified the body alone and must add
the timestamp prefix. Email is plain text and uses the same stable key in
`Message-ID`.

The initial attempt is followed by retries after 1 minute, 5 minutes, 15
minutes, 1 hour, 4 hours, and 12 hours. Webhook 408/425/429/5xx and network,
DNS, or TLS failures retry. SMTP 4xx and network failures retry; SMTP 5xx and
invalid configuration fail permanently. Manual retry preserves the stable
idempotency key.

A delivery that hits a storage error after it was picked up returns to
pending and is retried after one minute without counting as an attempt. A
delivery left in progress for longer than twice the delivery timeout (at least
one minute), for example because its worker could not record the result, is
returned to pending by a maintenance pass that runs every minute, so it never
blocks its channel until a restart. Deliveries are at least once: receivers
should use the idempotency key to drop duplicates.

## Outbound security and limits

DNS is checked at configuration and dial time. Environment proxies and webhook
redirects are disabled. Loopback, link-local, multicast, unspecified,
`.localhost`, cloud metadata targets and the reserved ranges `0.0.0.0/8`,
`192.0.0.0/24`, `198.18.0.0/15` and `240.0.0.0/4` are always blocked. Private
targets (RFC 1918, ULA, and carrier-grade NAT `100.64.0.0/10` as used by
Tailscale) require `BINNACLE_NOTIFICATIONS_ALLOW_PRIVATE_TARGETS=true` and a
restart. NAT64 (`64:ff9b::/96`), 6to4 (`2002::/16`) and IPv4-mapped or
IPv4-compatible addresses are unwrapped and judged by the IPv4 host they reach,
and the translated form itself counts as a private target.

| Environment variable | Default |
| --- | --- |
| `BINNACLE_NOTIFICATIONS_MAX_CONCURRENCY` | `4` |
| `BINNACLE_NOTIFICATIONS_QUEUE_CAPACITY` | `1000` |
| `BINNACLE_NOTIFICATIONS_DELIVERY_TIMEOUT` | `15s` |
| `BINNACLE_NOTIFICATIONS_REMINDER_INTERVAL` | `2h` |

Binnacle health (Settings → System) exposes queue depth, permanent failures, overflow deferrals,
and the last successful delivery. APIs, logs, and diagnostics never expose
destinations, credentials, recipients, decrypted secrets, or message bodies.
