# Checks and alerts operations

HTTP/HTTPS checks are outbound requests made by the Binnacle process. Public targets are allowed by default. Private-network targets are blocked unless the deployment sets `BINNACLE_CHECKS_ALLOW_PRIVATE_TARGETS=true` and restarts Binnacle. Loopback, link-local, multicast, unspecified, `.localhost`, and cloud-metadata targets remain blocked unconditionally.

The runner uses the shared outbound-network policy and does not use environment HTTP proxies. It validates DNS results, redirects, and the address actually dialled; follows at most three redirects; reads at most 64 KiB; and stores only a sanitized failure category. Response bodies are never persisted.

Checks are skipped while their resource is paused or archived. The worker count is bounded by `checks.max_concurrency` (default 8). The Alerts page retains raw alerts, built-in rules, checks, and timed silences. In v0.3, Incidents is the default view and notification channels deliver incident lifecycle events. See [Incidents and notifications](INCIDENTS_AND_NOTIFICATIONS.md).

Resolved alerts are retained for one year. Expired silences are retained for 90 days. Alert evaluation continues during a silence or deployment grace period, so a persistent failure fires when suppression ends.
