// The words of the room's head (docs/UI.md S3), from the session alone. Times are counted on the
// server's clock: `now` is the queue's serverTime moved on by the page (RoomHead).
import type { QueueSession } from "../../queue/types";

export type LiveState = "live" | "lastCall" | "off";

export function liveState(session: QueueSession | null): LiveState {
  if (!session?.live) return "off";
  return session.handsClosed ? "lastCall" : "live";
}

/** The label beside the repo's name. */
export const LIVE_LABEL: Readonly<Record<LiveState, string>> = {
  live: "Live",
  lastCall: "Last call",
  off: "Not live",
};

/** "Thursday office hours, live.": the session, then its state in blue italic serif. */
export type Headline = { readonly name: string; readonly state: string };

const STATE_WORDS: Readonly<Record<LiveState, string>> = {
  live: "live.",
  lastCall: "last call.",
  off: "ended.",
};

export function headline(session: QueueSession | null): Headline {
  if (session === null) return { name: "No session", state: "right now." };
  // "Ask me anything!" reads "Ask me anything, live.", not "Ask me anything!, live.".
  const title = session.title.replace(/[\s.,;:!?…]+$/u, "") || session.title;
  return { name: `${title},`, state: STATE_WORDS[liveState(session)] };
}

const MINUTE_MS = 60_000;

function minutesIn(session: QueueSession, now: number): number | null {
  const started = Date.parse(session.startedAt);
  if (!Number.isFinite(started) || !Number.isFinite(now)) return null;
  // A page whose clock is behind the server's must not count minutes before the start.
  return Math.max(0, Math.floor((now - started) / MINUTE_MS));
}

/** Minutes until the planned end, below zero once it is over; null when no length was planned. */
export function minutesLeft(session: QueueSession, now: number): number | null {
  const elapsed = minutesIn(session, now);
  if (elapsed === null || session.plannedMinutes === 0) return null;
  return session.plannedMinutes - elapsed;
}

function liveTime(session: QueueSession, now: number | null): string {
  if (now === null) return "";
  const left = minutesLeft(session, now);
  if (left === null) {
    const elapsed = minutesIn(session, now);
    return elapsed === null ? "" : `${elapsed} min in`;
  }
  if (left > 0) return `${left} min left`;
  return left === 0 ? "time is up" : `${-left} min over`;
}

function endedTime(session: QueueSession): string {
  const started = Date.parse(session.startedAt);
  const ended = Date.parse(session.endedAt ?? "");
  if (!Number.isFinite(started) || !Number.isFinite(ended)) return "ended";
  return `ended after ${Math.max(1, Math.round((ended - started) / MINUTE_MS))} min`;
}

/** The mono line under the headline: real state only. "" when there is no session to describe. */
export function metaLine(
  session: QueueSession | null,
  now: number | null,
): string {
  if (session === null) return "";
  const length =
    session.live && session.plannedMinutes > 0
      ? `${session.plannedMinutes} min`
      : "";
  const host = session.host ? `with @${session.host.login}` : "";
  const time = session.live ? liveTime(session, now) : endedTime(session);
  return [
    `Nº ${session.number}`,
    [length, host].filter(Boolean).join(" "),
    time,
  ]
    .filter(Boolean)
    .join(" · ");
}
