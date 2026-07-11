import { describe, expect, it } from 'vitest';
import { decodeSnapshot } from './live.svelte';

describe('live response decoding', () => {
  it('rejects malformed payloads instead of presenting them as current', () => {
    expect(() => decodeSnapshot('{"seq":1}')).toThrow(/malformed/);
    expect(
      decodeSnapshot(
        '{"seq":1,"ts":"2026-07-11T12:00:00Z","host":{},"resources":[],"collectors":{}}',
      ).seq,
    ).toBe(1);
  });
});
