# ADR 020: Product UI design system and information architecture

## Status

Accepted (2026-09-02)

## Context

The first web interface presented Binnacle as an "operations console": condensed
all-caps labels, square corners, a numbered bottom navigation bar, and terminal
vocabulary (roster, exceptions, logbook, commissioning). It showed point-in-time
numbers with no trend, kept seven unrelated alerting concerns in one component,
asked operators to type raw resource ids into forms, and styled everything
through one 1,860-line global stylesheet with no scoped styles. The product
promise, knowing what a server is doing before it runs out of room, was not
visible on the first screen.

## Decision

Binnacle ships a product-grade interface built on a small in-repo design
system rather than a console pastiche.

- **Visual language.** Dark by default with a complete light theme, cool
  neutrals, a teal accent reserved for interaction and brand, and five status
  tones (`ok`, `warn`, `critical`, `info`, `neutral`) that each carry text,
  background, border, and solid variants meeting WCAG 2.2 AA contrast in both
  themes. Geist Sans for interface text and Geist Mono with tabular numerals
  for every number, identifier, and log line. Both faces are vendored under
  the SIL Open Font License in `web/static/fonts/geist/`.
- **Tokens and primitives.** All colours, spacing, radii, shadows, motion, and
  layout constants live in `web/src/styles/tokens.css`; a short reset lives in
  `web/src/styles/base.css`. Every component carries its own scoped `<style>`
  block. Shared primitives in `web/src/lib/ui/` (buttons, badges, status pills,
  cards, stat tiles, sparklines, data table, tabs, segmented control, time
  range picker, dialogs, drawer-free menus, popover-based combobox, toasts,
  chart wrapper, command palette) are keyboard-complete and carry the ARIA the
  accessibility policy requires.
- **Dependencies.** Two runtime packages are added alongside `uplot`:
  `@lucide/svelte` (icons, ISC) and `@floating-ui/dom` (positioning, MIT).
  No CSS framework and no component kit; the interface still bundles into the
  single binary with no external requests.
- **Information architecture.** Left sidebar on desktop, bottom tab bar on
  phones, and a ⌘K command palette. Screens are Overview, Resources (list and
  detail), Host (metrics, filesystems, processes, collectors), Alerts
  (incidents, firing alerts, rules, checks, silences) with incident detail
  pages, Activity, Logs, and Settings split into General, Data & retention,
  Access, Notifications, Appearance, Integrations, and System. Notification
  channels and delivery history live under Settings because they are
  configuration; incidents link to their deliveries. Legacy paths (`/watch`,
  `/server`, `/events`, `/settings/monitor-health`, `/settings/diagnostics`)
  redirect client-side, and the stored `landingPage` preference migrates to the
  new names.
- **Headroom framing.** The Overview leads with open incidents and other
  attention items, then host headroom tiles with an hour of sparkline and
  per-mount disk usage, then resources grouped by project with per-row
  sparklines, then recent activity. Small additive API endpoints support this:
  `/api/v1/metrics/sparklines`, `/api/v1/filesystems` (and `filesystems` in the
  live snapshot), filter and cursor parameters on `/api/v1/events`, duration
  fields on alert rules, and the previously unreachable
  `/api/v1/resources/{id}` route.
- **Interaction rules.** Destructive actions confirm, the most dangerous with a
  typed phrase. Settings save explicitly with dirty state and toasts rather
  than on blur. Forms pick resources from a searchable combobox instead of
  free-text ids. Live streams never auto-scroll under the reader; new items
  are announced with a button.

## Consequences

- The Playwright suites, visual baselines, landing screenshot, and the demo
  generator were rewritten to match. Demo mode now produces projects,
  environments, multi-container resources, infrastructure resources, filesystem
  usage, and realistic event types so every screen can be exercised locally.
- The single global stylesheet and every console-era component were deleted.
  New screens must use the tokens and primitives; adding a colour outside
  `tokens.css` is a regression.
- Icons and fonts are pinned dependencies covered by the existing dependency
  policy in `docs/DEPENDENCIES.md`. Changing either is a design decision, not
  a routine upgrade.
