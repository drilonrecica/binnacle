# Supply-chain policy

Binnacle keeps a narrow dependency set and verifies it in CI.

## Automated gates

- **Go vulnerability scan** — `govulncheck ./...` runs on every PR/push.
- **Frontend audit** — `pnpm audit --audit-level moderate` runs on every PR/push.
- **License review** — `go-licenses` checks Go dependencies against the
  repository's explicit license allowlist.
- **SBOM** — Anchore's SBOM action generates an SPDX JSON SBOM on every push.
- **Container scan** — `trivy` scans the production image for HIGH/CRITICAL vulnerabilities on every push.

## Local targets

```bash
make vuln      # govulncheck + pnpm audit
make licenses  # go-licenses check
make sbom      # build image and generate SBOM
make scan      # build image and trivy scan
```

These targets require the corresponding local tools (`go-licenses`, `syft`,
`trivy`). CI performs equivalent checks using pinned workflow tools and actions.

## Response

- A critical or exploitable vulnerability in a production dependency blocks release until remediated or documented as a false positive.
- License findings outside the allowlist require explicit review. Incompatible
  dependencies must be replaced; an architectural licensing decision requires
  an ADR.
- SBOMs and scan results are retained as release qualification evidence.
