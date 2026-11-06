# Accessibility

Binnacle targets WCAG 2.2 Level AA. Automated checks run with `@axe-core/playwright` against the login, setup, onboarding, Headroom, Server, Resources (including the logs prompt), resource detail (every tab, Logs included), Changes, Incidents (every tab), incident detail, every Settings section, the command palette, dialogs, and the standalone landing page.

## Automated coverage

Run the accessibility smoke suite:

```bash
pnpm --dir web test:e2e a11y.spec.ts
pnpm --dir web test:landing
```

The tests scan each page with `wcag2a`, `wcag2aa`, `wcag21a`, and `wcag21aa` rules and fail on any violation.

## Manual checklist for releases

Verify each item in a supported browser with keyboard-only interaction and, where available, a screen reader.

### Page structure

- [ ] Skip-to-content link is the first focusable element and moves focus to `#content`.
- [ ] Each page has exactly one `<main>` landmark.
- [ ] Page `<title>` is "Binnacle — <page>" and route-specific context is conveyed by the first heading.
- [ ] Primary navigation uses `<nav aria-label="Primary navigation">` with Headroom, Resources, Server, Incidents, Changes and Settings in that order (Settings under "More" on phones), and marks the current page with `aria-current="page"`.
- [ ] Following a link to another page moves focus to `main#content`; switching a tab or filter (query-only change) leaves focus where it was.

### Login and setup

- [ ] Username and password fields have visible, programmatic labels.
- [ ] Focus moves to the error message after a failed submission.
- [ ] Password fields use `type="password"` and appropriate autocomplete tokens.

### Headroom

- [ ] Each instrument is a link to Server whose accessible name starts with the gauge's visible words ("CPU 42%, steady"; "Root disk / 88 GiB of 100 GiB, full in ~6 d, 88% used"); the gauge inside is a `meter` with min, max and value text.
- [ ] A gauge over its warn or critical line says "Above N%" next to a ring or square marker, not by colour alone.
- [ ] Each Incidents tab that fails to load shows an error with a retry (role `alert`), never an empty list; a failed refresh keeps the last rows under a stale notice.
- [ ] "Nothing needs attention" appears only while the stream is live and the incident list has loaded; otherwise the attention strip shows "Status unknown since hh:mm" with the last update.
- [ ] No boot id, container id or resource id appears as text on the page.

### Server

- [ ] The boot id appears once, in the title row, as a copy button whose accessible name starts with its visible text ("boot 6f1c…, copy the boot id").
- [ ] Instruments cover CPU, memory, swap and every filesystem; a disk's accessible name carries its time to full, and swap without a swap device says "not configured" instead of 0%.
- [ ] Every collector card states its state as a word and a reason; a collector that is not healthy is also named with its state in the title row.
- [ ] Redacted parts of process command lines are set apart as `[REDACTED]` text, not by colour alone.

### Color and visual information

- [ ] Status badges (healthy, degraded, down, unknown, archived) are readable with the text label; the colored dot is `aria-hidden`.
- [ ] Warning text meets 4.5:1 contrast on both light and dark themes.
- [ ] Charts are accompanied by a visible text summary and a keyboard-focusable inspector button.

### Interaction

- [ ] All buttons and links show a visible focus indicator.
- [ ] Dialogs trap focus while open and return focus to the triggering element on close.
- [ ] Disclosure widgets (details/summary) and tab-like navigation are keyboard operable.
- [ ] Forms can be completed and submitted using only the keyboard.

### Motion

- [ ] Enable `prefers-reduced-motion` and confirm transitions and animations are suppressed.
- [ ] Live updates do not auto-scroll or flash more than three times per second.

### Live regions

- [ ] Loading states use `role="status"`.
- [ ] Errors use `role="alert"`.
- [ ] Connection and deletion progress are announced politely.
- [ ] The connection status in the top bar is the only connection live region and announces only its state word (Live, Stale, Connecting, Disconnected); the "since" time and age next to it are not announced, and no clock sits in any live region.
