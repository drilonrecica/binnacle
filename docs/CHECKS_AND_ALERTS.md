# Checks and alerts operations

HTTP/HTTPS checks are outbound requests made by the Binnacle process. Public targets are allowed by default. Private-network targets are blocked unless the deployment sets `BINNACLE_CHECKS_ALLOW_PRIVATE_TARGETS=true` and restarts Binnacle. Loopback, link-local, multicast, unspecified, `.localhost`, and cloud-metadata targets remain blocked unconditionally.

The runner does not use environment HTTP proxies. It validates DNS results, redirects, and the address actually dialled; follows at most three redirects; reads at most 64 KiB; and stores only a sanitized failure category. Response bodies are never persisted.

Checks are skipped while their resource is paused or archived. The worker count is bounded by `checks.max_concurrency` (default 8). The Alerts page provides current alerts, built-in rules, checks, and timed silences. Built-in rules create local events only; v0.2 has no notification delivery or incident workflow.

Resolved alerts are retained for one year. Expired silences are retained for 90 days. Alert evaluation continues during a silence or deployment grace period, so a persistent failure fires when suppression ends.
