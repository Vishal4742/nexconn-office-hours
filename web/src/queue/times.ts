// Times as the panel shows them. Durations count on the server's clock (plan decision 12); a
// planned time is shown in the viewer's own time zone (FR-SESS-5).
import { toMs } from "./clock";

const MINUTE = 60_000;
const pad = (value: number) => String(value).padStart(2, "0");

/** How long a session has run: "20 min", or "1 h 05 min" from the first full hour. */
export function elapsedLabel(startedAt: string, serverNow: number): string {
  const ms = serverNow - toMs(startedAt);
  if (Number.isNaN(ms)) return "";
  const minutes = Math.max(0, Math.floor(ms / MINUTE));
  return minutes < 60
    ? `${minutes} min`
    : `${Math.floor(minutes / 60)} h ${pad(minutes % 60)} min`;
}

/**
 * A planned time, such as "Thu 19 Nov, 17:00", in the viewer's time zone. The locale and the zone
 * are the browser's unless given (tests give them).
 */
export function whenLabel(
  time: string,
  locale?: string,
  timeZone?: string,
): string {
  const ms = toMs(time);
  if (Number.isNaN(ms)) return "";
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
  }).format(ms);
}

/** The value of an <input type="datetime-local"> for a time: the viewer's wall clock, to the minute. */
export function toLocalInput(time: string): string {
  const ms = toMs(time);
  if (Number.isNaN(ms)) return "";
  const at = new Date(ms);
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

const LOCAL_INPUT =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::\d{2}(?:\.\d+)?)?$/;

/** The moment an <input type="datetime-local"> names on the viewer's wall clock, or null. */
export function fromLocalInput(value: string): Date | null {
  const parts = LOCAL_INPUT.exec(value);
  if (!parts) return null;
  const [year, month, day, hour, minute] = parts.slice(1).map(Number);
  const at = new Date(year, month - 1, day, hour, minute);
  return Number.isNaN(at.getTime()) ? null : at;
}
