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
- Never discard an error with `_` in `internal/`: return it, join it, or log
  it through the component's logger (background workers through their
  rate-limited `observe.ErrorLog`). `make go-vet` runs `go vet` and then
  `internal/lint/cmd/errdiscard`, which fails on any error assigned to `_`
  outside tests. Only calls documented never to fail (`crypto/rand.Read`,
  `hash.Hash.Write`, `bytes.Buffer` and `strings.Builder` writes) are exempt.

## Frontend tests

`make check` runs the Vitest unit and component tests. The Playwright suites
in `web/tests/e2e` run against the Vite dev server with every API mocked
(`fixtures.ts`), on a desktop and a phone project:

```bash
pnpm --dir web exec playwright install chromium   # once
pnpm --dir web test:e2e                           # everything
pnpm --dir web test:e2e live.spec.ts              # one suite
```

- One spec per area (`shell`, `headroom`, `resources`, `server`,
  `incidents`, `changes`, `settings`, `entry`), plus `live.spec.ts` for the
  stream, `a11y.spec.ts` for axe in both themes and `visual.spec.ts` for the
  screenshots.
- The mocked live stream stays open like the real one. A spec can send
  frames with `emitLive` and end the connection with `dropLive` to test
  disconnects and session expiry.
- `visual.spec.ts` holds a baseline per screen, theme and width. After an
  intended UI change, regenerate them with
  `pnpm --dir web exec playwright test visual.spec.ts --update-snapshots`
  and review every changed image in the pull request.
- The README, `docs/assets/screenshots/` and the landing page use real
  captures of the demo, not the mocked baselines. After a visible UI change,
  run `make build && pnpm --dir web screenshots`, which starts
  `binnacle --demo` on a temporary database, signs in and captures each
  screen (about five minutes, because the time-to-full estimate needs live
  samples). The demo is live, so values differ between runs; review the
  images, then refresh the landing visual baseline with
  `pnpm --dir web test:landing --update-snapshots`.

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
