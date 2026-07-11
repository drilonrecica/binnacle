export const maxChartPoints = 1000;
export type Point = { at: number; value: number | null };
export function toSeries(
  points: Point[],
  cap = maxChartPoints,
): [number[], (number | null)[]] {
  const trimmed = points.slice(-cap);
  return [
    trimmed.map((point) => point.at),
    trimmed.map((point) => point.value),
  ];
}
export function summary(points: Point[]) {
  const values = points.flatMap((point) =>
    point.value == null ? [] : [point.value],
  );
  return values.length
    ? {
        min: Math.min(...values),
        avg: values.reduce((a, b) => a + b, 0) / values.length,
        max: Math.max(...values),
      }
    : null;
}
