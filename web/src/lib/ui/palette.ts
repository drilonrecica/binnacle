export interface PaletteItem {
  id: string;
  group: string;
  label: string;
  hint?: string;
  keywords?: string[];
  href?: string;
  action?: () => void;
}

/**
 * Scores how well `query` matches `text`. Higher is better; 0 means no match.
 * Prefix and word-start matches beat scattered subsequence matches.
 */
export function scoreMatch(query: string, text: string): number {
  const needle = query.trim().toLowerCase();
  if (!needle) return 1;
  const haystack = text.toLowerCase();
  if (haystack === needle) return 100;
  if (haystack.startsWith(needle)) return 80;
  const wordIndex = haystack.indexOf(` ${needle}`);
  if (wordIndex >= 0) return 70;
  const index = haystack.indexOf(needle);
  if (index >= 0) return 50 - Math.min(20, index);
  // Subsequence match: every character in order.
  let position = 0;
  let gaps = 0;
  for (const char of needle) {
    const found = haystack.indexOf(char, position);
    if (found < 0) return 0;
    gaps += found - position;
    position = found + 1;
  }
  return Math.max(1, 25 - Math.min(20, gaps));
}

export function rankItems<T extends PaletteItem>(
  items: T[],
  query: string,
  limit = 12,
): T[] {
  const needle = query.trim();
  if (!needle) return items.slice(0, limit);
  return items
    .map((item) => {
      const scores = [
        scoreMatch(needle, item.label),
        ...(item.keywords ?? []).map(
          (keyword) => scoreMatch(needle, keyword) * 0.9,
        ),
        item.hint ? scoreMatch(needle, item.hint) * 0.6 : 0,
      ];
      return { item, score: Math.max(...scores) };
    })
    .filter((entry) => entry.score > 0)
    .sort(
      (a, b) => b.score - a.score || a.item.label.localeCompare(b.item.label),
    )
    .slice(0, limit)
    .map((entry) => entry.item);
}
