# 002 — Svelte 5 frontend with runes and TypeScript

## Context

The embedded UI needs a compact, typed browser build.

## Decision

Use Svelte 5 runes and TypeScript. See [ADR 008](008-single-binary.md).

## Consequences

The production process has no Node.js runtime.

## Alternatives

Large component frameworks are not adopted.
