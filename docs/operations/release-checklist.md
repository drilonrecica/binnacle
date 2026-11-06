# Release checklist

This checklist covers the objective gates required before publishing
the current candidate.

## Automated gate

Run the full release gate:

```bash
./scripts/release-gate.sh
```

It produces `release-record/<version>-<short-sha>.md` with a pass/fail
table and captured benchmark output.

## Required gates

| Gate | Why it matters | Reject if |
| --- | --- | --- |
| `make check` | Local CI-quality subset (format, vet, tests, lint) | Any check fails |
| License and security policy | Legal and responsible disclosure baseline | `LICENSE` or `SECURITY.md` missing |
| Binary build | Production artifact compiles | Build error |
| Compose and Coolify validation | Deployment settings and templates agree | Template validation fails |
| Socket proxy policy | Only required Docker reads pass; mutation and archive paths are denied | Raw socket reaches Binnacle or a forbidden path succeeds |
| Container image build | Installation artifacts exist | Image build fails |
| Demo container smoke | Unauthenticated liveness responds from the locally built candidate image | `/healthz` fails |
| Benchmark (long-run) | 30 and 100 containers on 48 h of seeded raw history with checks, eight SSE clients and a retention pass stay within the targets in `docs/operations/benchmarking.md` | Any target missed or not measured (`make benchmark` exits 1) |
| Security and integration race tests | Auth, tokens, enrichment, diagnostics, exports, storage, and preferences remain race-free | Any targeted race test fails |
| Browser and accessibility suites | Access, diagnostics, token, preference, mobile, and accessibility workflows remain usable | Playwright or visual regression fails |
| Advanced-auth acceptance | Default-hidden UI/routes, enabled TOTP/proxy workflows, and stored-enrollment startup refusal are verified | Any advanced-auth acceptance case fails |
| Portability acceptance | Default-hidden UI/routes, session-only reads, enabled token/export/Prometheus workflows, mobile, and accessibility cases are verified | Any portability acceptance case fails |

## Optional gates

| Gate | Notes |
| --- | --- |
| Supply-chain scan | `make vuln` (uses the same accepted-risk wrapper as CI; requires network, `jq`, and `govulncheck`) |
| Real-host validation | `scripts/validate-host.sh --window 30` under steady load on each target host class; no persistent `CHECK` rows (procedure in [metric-validation.md](metric-validation.md#real-host-procedure)) |
| Coolify fresh install | Deploy `packaging/coolify/binnacle.yaml` to a Coolify instance |
| Compose fresh install | Set `BINNACLE_IMAGE=ghcr.io/drilonrecica/binnacle:local`, then run `docker compose -f packaging/docker/docker-compose.yml up` |
| Upgrade test | Install previous build, persist data, upgrade to candidate |
| Retention / persistence failure | Fill queue, verify drops are bounded and data recovers |

## Go/no-go rules

- **GO:** All required gates pass. Optional gates may be skipped only with a
  documented reason. Minor visual defects are acceptable if recorded.
- **NO-GO:** Any critical security defect, normal-operation data loss, or
  required gate failure remains.

Packaged defaults enable `BINNACLE_FEATURE_ADVANCED_AUTH` and
`BINNACLE_FEATURE_PORTABILITY` because both acceptance suites
(`make acceptance-auth`, `make acceptance-portability`) run in CI and in
`scripts/release-gate.sh`. If either suite fails, the release is NO-GO.

## Evidence retention

Attach the following to the release record:

1. `release-record/build.log`
2. `release-record/<version>-<short-sha>.md`
3. `benchmark-report.json` and `benchmark-report.md`
4. Container image digest (`docker inspect --format='{{index .RepoDigests 0}}' ghcr.io/drilonrecica/binnacle:local`)
5. E2E and visual regression reports when run
