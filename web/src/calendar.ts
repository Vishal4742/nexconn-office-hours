// Putting a session in someone's calendar (FR-SESS-5): Google Calendar's own form, the server's
// feed (an .ics file that Apple Calendar and Outlook open), and a webcal:// address to subscribe
// to every session of a project. Plain logic, so it can be tested.
import { roomPath } from "./projects";
import type { RepoName } from "./projects";
import { toMs } from "./queue/clock";

/** The length a session without a plan is drawn for, as on the server. */
const DEFAULT_MINUTES = 45;

/** The project's sessions as an iCalendar feed, on our own origin (server/calendar.go). */
export function calendarPath(repo: RepoName): string {
  return `${roomPath(repo)}/calendar.ics`;
}

/** The feed as an address calendar apps subscribe to: they ask again every hour. */
export function subscribeUrl(repo: RepoName, origin: string): string {
  return `${origin.replace(/^https?:/, "webcal:")}${calendarPath(repo)}`;
}

/** 20261119T113000Z: how Google Calendar writes a moment in UTC. */
function utc(ms: number): string {
  return new Date(ms).toISOString().replace(/[-:]/g, "").replace(/\.\d+/, "");
}

/**
 * Google Calendar's form for a new event, filled in with the session. "" for a time that can't
 * be read.
 */
export function googleCalendarUrl(
  repo: RepoName,
  startsAt: string,
  minutes: number,
  origin: string,
): string {
  const start = toMs(startsAt);
  if (Number.isNaN(start)) return "";
  const end = start + (minutes > 0 ? minutes : DEFAULT_MINUTES) * 60_000;
  const room = `${origin}${roomPath(repo)}`;
  const query = new URLSearchParams({
    action: "TEMPLATE",
    text: `Office hours: ${repo.owner}/${repo.repo}`,
    dates: `${utc(start)}/${utc(end)}`,
    details: `Live office hours with the maintainers. Raise your hand in the room: ${room}`,
    location: room,
  });
  return `https://calendar.google.com/calendar/render?${query.toString()}`;
}
