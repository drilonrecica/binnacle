# Contributing to Binnacle

Binnacle is a founder-led AGPL-3.0-only project. Contributions are welcome when
they fit the product boundaries and keep the monitor lightweight, local
first, and permanently read-only with respect to monitored workloads.

## Before starting

Read the [product boundaries](docs/PRODUCT.md), [roadmap](ROADMAP.md), and
relevant [architecture decisions](adr/). Product boundaries and accepted ADRs
are authoritative; the roadmap is directional. Do not add an external database,
telemetry, generic Docker control features, or roadmap scope without a recorded
product decision. Implementation work belongs in issues, milestones, or
release-specific plans rather than a permanent repository backlog.

For a non-trivial architectural change, open an issue or proposal before
writing a large patch. See [GOVERNANCE.md](GOVERNANCE.md) for the ADR process.

## Development expectations

- Keep changes focused and preserve existing behavior unless a change is
  explicitly intended.
- Add behavioral tests for regression-prone logic and run the relevant Make
  targets before opening a pull request.
- Do not commit secrets, production databases, generated frontend assets, or
  local profiles.
- Use clear, accessible UI behavior and avoid adding large dependencies without
  an ADR.
- Do not expose Docker mutation endpoints or proxy arbitrary Docker API calls.

## Pull requests

Use a focused branch and explain the problem, approach, tests, and any
follow-up work. Small fixes may be reviewed directly. Major architecture,
security, persistence, API compatibility, deployment, frontend-architecture,
or licensing changes require an ADR or lightweight RFC first.

Contributions are normally squash-merged. Maintainers may request changes,
split a proposal, or defer work that does not fit the active release scope.

## Commit convention and DCO

Use Conventional Commits:

```text
feat: add capability
fix: correct behavior
perf: reduce hot-path overhead
refactor: restructure without changing behavior
test: add or correct tests
docs: update documentation
build: change build tooling
ci: change automation
chore: maintain repository metadata
```

Every commit must include Developer Certificate of Origin sign-off:

```text
Signed-off-by: Your Name <your-email@example.com>
```

Use `git commit -s` to add it. By signing off, you certify that you have the
right to submit the contribution under the repository license, as described by
the [Developer Certificate of Origin 1.1](https://developercertificate.org/).

## Reporting security issues

Do not open a public issue for a suspected vulnerability. Follow
[SECURITY.md](SECURITY.md).
