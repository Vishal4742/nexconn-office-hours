import { calendarPath, googleCalendarUrl, subscribeUrl } from "../calendar";
import type { RepoName } from "../projects";

type Props = {
  readonly repo: RepoName;
  /** When the session starts, as the server wrote it. */
  readonly startsAt: string;
  /** Its planned length; 0 draws 45 minutes. */
  readonly minutes?: number;
};

/**
 * FR-SESS-5: put the next session in a calendar. Google Calendar opens its own form; the .ics
 * file opens in Apple Calendar and Outlook; subscribing follows every session of the project.
 */
export function AddToCalendar({ repo, startsAt, minutes = 0 }: Props) {
  const origin = window.location.origin;
  const google = googleCalendarUrl(repo, startsAt, minutes, origin);
  if (!google) return null;
  return (
    <p className="add-cal">
      <span className="add-cal-label">Add to calendar:</span>{" "}
      <a href={google} target="_blank" rel="noopener noreferrer">
        Google
      </a>
      {" · "}
      <a href={calendarPath(repo)}>Apple or Outlook</a>
      {" · "}
      <a href={subscribeUrl(repo, origin)}>Subscribe</a>
    </p>
  );
}
