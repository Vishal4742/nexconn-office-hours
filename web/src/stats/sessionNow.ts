// What the join page says about a project's sessions (FR-PROJ-3, FR-SESS-5): one that is live,
// the next one's time, or that none is planned. Plain logic, so it can be tested.
import { toMs } from "../queue/clock";
import type { Queue } from "../queue/types";

export type SessionNow =
  | { readonly kind: "live"; readonly title: string }
  /** `at` as the server sent it, `when` as the viewer reads it. */
  | { readonly kind: "next"; readonly at: string; readonly when: string }
  | { readonly kind: "none" };

/**
 * FR-SESS-5: a moment in the viewer's own time zone and way of writing dates, "Thu 19 Nov, 17:00"
 * or "Thu, Nov 19, 5:00 PM". The locale and zone are the browser's unless given. "" when the
 * moment can't be read.
 */
export function sessionTime(
  iso: string,
  locale?: string,
  timeZone?: string,
): string {
  const at = toMs(iso); // Go writes up to nine digits after the second
  if (Number.isNaN(at)) return "";
  // A 24-hour clock writes 09:30, a 12-hour one 9:30 AM.
  const { hour12 } = new Intl.DateTimeFormat(locale, {
    hour: "numeric",
    timeZone,
  }).resolvedOptions();
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: hour12 ? "numeric" : "2-digit",
    minute: "2-digit",
    timeZone,
  }).format(at);
}

/**
 * The session line of the join page. A planned time that has passed is not the next session: the
 * server keeps it until a maintainer changes it. "Passed" is counted on the server's clock.
 */
export function sessionNow(
  queue: Queue,
  locale?: string,
  timeZone?: string,
): SessionNow {
  const { session, nextSessionAt, serverTime } = queue;
  if (session?.live) return { kind: "live", title: session.title };
  if (!nextSessionAt) return { kind: "none" };
  const isAhead = toMs(nextSessionAt) > toMs(serverTime); // false for NaN too
  const when = isAhead ? sessionTime(nextSessionAt, locale, timeZone) : "";
  return when ? { kind: "next", at: nextSessionAt, when } : { kind: "none" };
}
