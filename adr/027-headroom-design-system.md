# ADR 027: Headroom design system

## Status

Accepted (2026-11-05). Supersedes the visual-language, token and font parts of
ADR 020; its information architecture, dependency and interaction rules stand
until the P4 tasks that replace them land. Implemented by P4-01 of the v1.0
plan from `imp-docs/DESIGN.md` (which calls this ADR "023"; that number was
already taken by the master-key ADR, so it is 027).

## Context

ADR 020 gave Binnacle a competent but interchangeable developer-SaaS look:
cool navy neutrals, a teal accent, Geist. The first screen answered "what needs
attention" with point-in-time numbers. The owner's brief is different: the
product exists to say how much room is left and when it runs out. The redesign
("Headroom", decided with the owner on 2026-11-05) makes the interface an
instrument for that question and gives the product a brand of its own. A
binnacle keeps a ship's compass readable; the compass card is the mark, the
arc gauge is the instrument, and brass on slate is the material.

The tokens in `web/src/styles/tokens.css` are the contract every component
builds on, so they change first, with a machine check, and the primitives and
pages follow in P4-03 onward.

## Decision

1. **Palette.** Dark first; light is a full theme, not an inversion. Dark:
   `--bg` #1f242b, `--panel` #272d35, `--panel-2` #2b3139 (hover, nested
   surfaces), `--line` #363d47, `--fg` #e9e4d8, `--muted` #9aa1ab. Light:
   #f3f1ec, #ffffff, #f9f8f5, #d9d6cf, #1f242b, #5f6873. Brass `--accent`
   #d2a24c (light #9a6b1e) is reserved for needles, links, focus rings,
   primary buttons and trend text; nothing else is brass.
2. **Status tones with shape.** `ok`, `warn`, `critical`, `info` and `neutral`
   each carry `-fg` (text), `-bg` (10 percent tint), `-border`, `-solid` and
   `-shape` tokens. The shapes are circle (ok), ring (warn), square
   (critical), diamond outline (info) and dashed circle (neutral or unknown),
   and every marker is accompanied by a word or an accessible name, so status
   never depends on colour alone. Solid values are the spec's hues: dark
   #8fbf9f, #e0b25a, #e2735e, #8fb3d9; light #2f6b4f, #b7791f, #b23a2a,
   #2f5d8f. Warn in light is its own amber, never the brass accent.
3. **Text variants differ from solids where the spec hue fails AA.** Measured
   with the WCAG formula, the spec's single value per tone does not reach
   4.5:1 everywhere: dark critical #e2735e on `--panel-2` is 4.28:1; light
   brass #9a6b1e on `--bg` is 4.14:1; light warn #b7791f is 3.2 to 3.6:1 on
   every light surface. So `--critical-fg` (dark) is #ec8e79, `--accent-text`
   (light) is #7f5717 and `--warn-fg` (light) is #8a5a10, while the solid,
   marker and button fills keep the spec values (3:1 is enough for them).
   `--muted` is one step lighter than the spec's #9aa1ab and #5f6873
   (#a4abb5 dark, #5a636e light) because notices and selected cards put muted
   text on a tone's tint, where the spec value measures 4.1 to 4.4:1; the
   neutral tone's text is `--muted`, as the spec says. Brass on slate measures
   6.7:1, not the 7.9:1 the design note estimated.
4. **Chart palette.** Six series distinct from the status tones: brass,
   steel, sage, lilac, sand, rose. Dark uses the spec pastels. The same
   pastels are 1.5 to 2.2:1 on light surfaces, so light uses darker members
   of the same hues (#9a6b1e, #3b6ea5, #4a7d58, #6a5bb8, #8a6d3b, #a04f6e),
   each at least 4.2:1. The first series of any chart is brass; red and green
   are never series colours.
5. **Contrast is a build gate.** `web/scripts/contrast.mjs` parses the token
   file, resolves `var()` chains, composites tints over the surfaces they sit
   on and checks 101 pairs per theme: text on every surface, tone text on
   its own tint, body and muted text on every tint, all at 4.5:1; focus
   rings, markers and series at 3:1. It runs in
   `make web-check` and exits non-zero on the first regression; a vitest suite
   covers its maths, parser and the committed file.
6. **Type.** Barlow Condensed 600 for the h1 (26 px) and gauge values (32 and
   36 px, tabular numerals); Barlow 400/500/600 for text at 14 px body and
   12.5 px meta, line-height 1.4; JetBrains Mono 400/500 for timestamps, ids,
   table numbers (12 px), log lines (13 px) and code. Uppercase with 0.06 em
   tracking only for instrument labels and section eyebrows. All three faces
   are vendored as static woff2 per weight under the SIL Open Font License in
   `web/static/fonts/<family>/` with that family's `OFL.txt` beside the files;
   Geist is removed from the app (the landing page keeps its copy until
   P4-15). Static instances rather than variable fonts because Barlow has no
   official variable build and a weight range on a static file renders faux
   bold. Provenance: Barlow and Barlow Condensed from google/fonts at commit
   24ddc1f324cac5d9f5aedca126d298f56e3f1640 (`ofl/barlow`,
   `ofl/barlowcondensed`), converted with
   `pyftsubset <ttf> --flavor=woff2 --unicodes='*' --layout-features='*'
   --glyph-names --notdef-outline --name-IDs='*'` (every glyph and feature
   kept, format only); JetBrains Mono from release v2.304
   (`fonts/webfonts/*.woff2`, unmodified).
7. **Shape, space, breakpoints, motion.** Radius 4 px on controls and chips,
   6 px on cards, no pills. Spacing 4, 8, 12, 16, 24, 32, 48 as
   `--space-{1,2,3,4,6,8,12}` (name is px over 4). Breakpoints 560, 900 and
   1200 px and nothing else; the sidebar becomes a tab bar below 900. Borders
   over shadows, with one shadow for dialogs and menus. 150 ms ease-out for
   state changes, 300 ms for needles, both zero under reduced motion.
8. **Compatibility aliases, then deletion.** The token file ends with a block
   that maps every ADR 020 name the current components still read
   (`--surface`, `--text-2`, `--n-*`, `--radius-full`, and so on) onto the new
   tokens, so the app renders in the new palette before the primitives are
   rebuilt. P4-03 deletes that block; nothing new may use a name from it.

## Consequences

- Every primitive and page is restyled on these tokens by P4-03 to P4-12; until
  then the app is the old layout in the new palette and faces. The Playwright
  visual baselines no longer match and are regenerated by P4-14.
- Seven font files (about 360 KB, 180 KB of it JetBrains Mono's full glyph
  set) replace two (140 KB). They are served with an ETag and revalidate
  with a 304; only hashed `/assets/` get the immutable header.
- A colour outside `tokens.css` remains a regression, and a token change
  that drops below AA now fails `make check` rather than an audit.
- Component media queries still use ten different widths; they move to the
  three breakpoints as each screen is rebuilt.

## Alternatives

- Keep Geist and change only colours: a skin on the old hierarchy, which the
  design brief lists as a failure mode.
- Edit ADR 020 in place: accepted ADRs are never edited here; superseding
  keeps the history of why the teal system existed.
- Hash the fonts through Vite for immutable caching: would move them out of
  `web/static/fonts/`, where the design spec and the dependency inventory
  point; revisit if font revalidation ever shows up in traces.
