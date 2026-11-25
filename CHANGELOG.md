# Changelog

All notable changes to Binnacle are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Domain sweep (ADR 028): a list of the operator's public domains, independent
  of Docker resources, probed on an interval (10 minutes to 24 hours, one hour
  by default) for NS and A/AAAA records, the final status of
  `GET https://<domain>/`, certificate issuer and expiry, the
  `Strict-Transport-Security` and `Content-Security-Policy` headers, the SPF
  record and the DMARC policy. A fingerprint change between two runs writes a
  `domain_change` event to the Changes page.
- Five alert families for swept domains: `domain_unreachable`,
  `domain_cert_expiring` (14 days), `domain_dns_missing`,
  `domain_mail_auth_missing` and `domain_headers_missing`.
- `/api/v1/sweep/domains` routes, a Domains card under Settings › Alerting,
  the `[sweep]` settings section (`BINNACLE_SWEEP_ENABLED`,
  `BINNACLE_SWEEP_MAX_CONCURRENCY`, `BINNACLE_SWEEP_DEFAULT_INTERVAL`) and
  seven demo domains.

### Fixed

- Alert resolution failed on every tick with a duplicate `events.id`, because
  the `alert_triggered` and `alert_resolved` events of one alert shared an id;
  a firing alert never resolved.
