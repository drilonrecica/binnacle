# Checks and alerts operations

HTTP/HTTPS checks are outbound requests made by the Binnacle process. Public targets are allowed by default. Private-network targets (RFC 1918, ULA, and carrier-grade NAT `100.64.0.0/10`) are blocked unless the deployment sets `BINNACLE_CHECKS_ALLOW_PRIVATE_TARGETS=true` (`checks.allow_private_targets`, shown in Settings → System and in diagnostics bundles) and restarts Binnacle. Loopback, link-local, multicast, unspecified, `.localhost`, cloud-metadata targets and the reserved ranges `0.0.0.0/8`, `192.0.0.0/24`, `198.18.0.0/15` and `240.0.0.0/4` remain blocked unconditionally. NAT64 and 6to4 addresses are unwrapped and judged by the IPv4 host they reach.

The runner uses the shared outbound-network policy and does not use environment HTTP proxies. It validates DNS results, redirects, and the address actually dialled; follows at most three redirects; reads at most 64 KiB; and stores only a sanitized failure category. Response bodies are never persisted.

Checks are skipped while their resource is paused or archived. The worker count is bounded by `checks.max_concurrency` (default 8). The Alerts page opens on Incidents and also lists firing alerts, built-in rules, HTTP checks, and timed silences; notification channels are configured under Settings → Notifications and deliver incident lifecycle events. See [Incidents and notifications](INCIDENTS_AND_NOTIFICATIONS.md).

Resolved alerts are retained for one year. Expired silences are retained for 90 days. Alert evaluation continues during a silence or deployment grace period, so a persistent failure fires when suppression ends.
