import { describe, expect, it } from 'vitest';
import { toSeries } from './chart';
describe('chart conversion', () => {
  it('keeps gaps and caps data', () =>
    expect(
      toSeries(
        [
          { at: 1, value: null },
          { at: 2, value: 3 },
        ],
        1,
      ),
    ).toEqual([[2], [3]]));
});
