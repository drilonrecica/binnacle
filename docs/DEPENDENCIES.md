# Dependency policy and inventory

Binnacle keeps its runtime and build dependency sets narrow. Manifest and
workflow files are the authoritative inventories; this document records how to
review them without duplicating version numbers that quickly become stale.

Last reviewed: 2026-11-05

## Sources of truth

| Area | Authoritative files |
| --- | --- |
| Go toolchain and modules | [`go.mod`](../go.mod), [`go.sum`](../go.sum) |
| Frontend toolchain and packages | [`package.json`](../package.json), [`web/package.json`](../web/package.json), [`pnpm-lock.yaml`](../pnpm-lock.yaml) |
| Container base and sidecar images | [`packaging/docker/Dockerfile`](../packaging/docker/Dockerfile), [`packaging/docker/docker-compose.yml`](../packaging/docker/docker-compose.yml) |
| GitHub Actions | [`.github/workflows/`](../.github/workflows/) |
| Automated bumps | [`renovate.json`](../renovate.json) |
| Vendored fonts and licenses | [`web/static/fonts/`](../web/static/fonts/), [`landing/assets/fonts/`](../landing/assets/fonts/) |

The app vendors Barlow, Barlow Condensed and JetBrains Mono (all SIL OFL, see
ADR 027) under `web/static/fonts/`; the landing page still carries Geist until
it is restyled.

Production uses a Go binary, SQLite through CGO, and embedded frontend assets.
Node.js and frontend packages are build-time dependencies only. Exact versions
must be read from the files above rather than copied into operational guides.
Docker integration uses the stable split Moby API and client modules rather
than the monolithic daemon module. The production runtime requires Docker
Engine 29.5.1 or newer; update the host before deploying Binnacle.
Production packaging also pins `wollomatic/socket-proxy` by release and digest.
It is the only container that receives the raw daemon socket and exposes only
Binnacle's required read paths through a private filtered Unix socket.

## Policy

- Pin reproducible toolchain, module, package, container, and CI inputs.
- Apply security updates promptly; take major upgrades during a deliberate
  compatibility pass rather than automatically.
- Add a dependency only when the maintained code it replaces would be riskier
  or materially more complex.
- Require an AGPL-3.0-compatible license. A new license category requires
  explicit review.
- Pin every container base image (`FROM`) and sidecar image by `@sha256:`
  digest, keeping the tag in front of it for readability.
- Pin every GitHub Action by full 40-character commit SHA with the release
  tag in a trailing comment (`uses: owner/repo@<sha> # v1.2.3`). Tags are
  mutable; release jobs hold `packages: write`, so a moved tag is a
  supply-chain path.
- Set `web/package.json` ranges to a floor at the locked minor or patch
  (for example `^5.56.4`), never `^major.0.0`; `pnpm-lock.yaml` fixes the
  exact version and CI installs with `--frozen-lockfile`.
- Let Renovate ([`renovate.json`](../renovate.json)) propose bumps for
  digests, action SHAs, Go modules and npm packages. Digest and SHA bumps
  are reviewed like any other change; major updates wait for approval on the
  dependency dashboard, and security updates are raised immediately.
- Record temporary compatibility exceptions next to the relevant manifest or
  in an ADR when they affect architecture.

## Local review

```bash
go list -m all
pnpm --dir web list --depth=0
make vuln
make licenses
make sbom
make scan
```

The last four targets require network access or their documented external
tools. CI runs vulnerability, license, SBOM, and container scan workflows; see
[the supply-chain policy](operations/supply-chain.md).
