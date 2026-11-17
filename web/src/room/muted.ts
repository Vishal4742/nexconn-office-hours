import { toMs } from "../queue/clock";

/**
 * What a muted member reads above the chat (FR-MOD-1), or null while no mute runs. `now` is the
 * server's clock, which also ended the mute; the time is in the viewer's own zone unless a locale
 * and a zone are given (tests give them).
 */
export function mutedLine(
  until: string | null,
  now: number,
  locale?: string,
  timeZone?: string,
): string | null {
  const ends = until === null ? Number.NaN : toMs(until);
  if (!(ends > now)) return null;
  const time = new Intl.DateTimeFormat(locale, {
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
  }).format(ends);
  return `A maintainer muted you in this room until ${time}. You can still read it.`;
}
