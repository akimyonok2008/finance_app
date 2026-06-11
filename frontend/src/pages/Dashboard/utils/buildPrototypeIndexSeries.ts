// TODO: Replace this with GET /portfolio/index-history once the backend exposes historical snapshots.

export type IndexPoint = {
  label: string;
  index: number;
};

/**
 * Build a deterministic 7-point prototype chart series ending at `currentIndex`.
 * The path is derived from the index value itself so it is stable across renders
 * (not random). When currentIndex ≈ 100 the series stays flat to avoid a
 * misleading upward trend.
 */
export function buildPrototypeIndexSeries(currentIndex: number): IndexPoint[] {
  const end = Number.isFinite(currentIndex) ? currentIndex : 100;
  const gain = end - 100;

  // Deterministic "seed" values derived from the gain so the curve shape
  // reflects the direction of performance.
  const offsets =
    gain >= 0
      ? [0, 0.08, 0.18, 0.32, 0.52, 0.74, 1]
      : [0, 0.12, 0.22, 0.38, 0.55, 0.78, 1];

  const points: IndexPoint[] = offsets.map((t, i) => {
    // Add a tiny deterministic wobble (no Math.random) to make the chart look
    // natural rather than perfectly linear.
    const wobble = ((i * 7 + Math.floor(Math.abs(gain) * 3)) % 5) * 0.04 * gain;
    const value = 100 + gain * t + wobble;

    const label =
      i === 0 ? "Baseline" : i === offsets.length - 1 ? "Now" : "";
    return { label, index: Math.round(value * 100) / 100 };
  });

  // Ensure the final point is exactly the current index.
  points[points.length - 1].index = end;

  return points;
}
