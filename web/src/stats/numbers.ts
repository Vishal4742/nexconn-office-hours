// How the stats pages word the five numbers of FR-MET-1. Plain logic, so it can be tested.
import type { Stats } from "../stats";

export type StatTile = {
  readonly key: keyof Stats;
  /** Grouped the way the viewer writes numbers: 1,234 or 1.234. */
  readonly value: string;
  readonly name: string;
};

/** In the order of FR-MET-1, each with its name for one and for any other count. */
const NAMES: readonly (readonly [keyof Stats, string, string])[] = [
  ["sessions", "session held", "sessions held"],
  ["handsRaised", "hand raised", "hands raised"],
  ["handsAnswered", "hand answered", "hands answered"],
  ["faqEntries", "FAQ entry", "FAQ entries"],
  ["participants", "participant", "participants"],
];

export function statTiles(stats: Stats, locale?: string): readonly StatTile[] {
  const format = new Intl.NumberFormat(locale);
  return NAMES.map(([key, one, many]) => ({
    key,
    value: format.format(stats[key]),
    name: stats[key] === 1 ? one : many,
  }));
}

/**
 * Before the first session there is nothing to count: every hand and every answer belongs to a
 * session. The people who joined a room (its maintainer at least) are no reason for a page of 0s.
 */
export function hasNothingYet(stats: Stats): boolean {
  return stats.sessions === 0;
}
