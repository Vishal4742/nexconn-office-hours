import { useId, useState } from "react";
import type { FormEvent } from "react";

import { AddToCalendar } from "../../components/AddToCalendar";
import { Sheet } from "../../components/Sheet";
import type { Project } from "../../projects";
import { setNextSession, startSession } from "../../queue/api";
import { serverNow, useQueue } from "../../queue/store";
import { fromLocalInput, toLocalInput, whenLabel } from "../../queue/times";
import {
  SESSION_TITLE,
  charCount,
  cleanLine,
  lengthProblem,
  scheduleProblem,
} from "./limits";
import { Refused, TextField, useQueueSend } from "./parts";

type Props = {
  readonly project: Project;
  readonly onClose: () => void;
};

/** The planned lengths a maintainer can choose, in minutes; 0 is "no set length" (FR-SESS-1). */
const LENGTHS = [0, 15, 30, 45, 60, 90, 120, 180] as const;
const USUAL_LENGTH = 45;

/** "Thursday office hours": what most sessions are called, on the day they start. */
function usualTitle(): string {
  const day = new Date().toLocaleDateString("en-US", { weekday: "long" });
  return `${day} office hours`;
}

/**
 * A maintainer starts a session now, with a title and an optional length (FR-SESS-1), or plans
 * one for later (FR-SESS-5), as Meet makes scheduling part of starting a meeting. Going live gives
 * the answer to acceptQueue and closes; planning shows the time with its calendar links. The props
 * are a contract (notes/phase-2-plan.md).
 */
export function StartSessionSheet({ project, onClose }: Props) {
  const id = useId();
  const queue = useQueue();
  const plannedAt = queue.status === "ready" ? queue.queue.nextSessionAt : null;
  const [isLater, setIsLater] = useState(false);
  const [title, setTitle] = useState(usualTitle);
  const [minutes, setMinutes] = useState<number>(USUAL_LENGTH);
  const [laterValue, setLaterValue] = useState(() =>
    plannedAt ? toLocalInput(plannedAt) : "",
  );
  const [scheduled, setScheduled] = useState<string | null>(null);
  const [hasTried, setHasTried] = useState(false);
  // Checked in the handlers against the server's clock, which also judges it; a render stays pure.
  const [timeProblem, setTimeProblem] = useState<string | null>(null);
  const [sending, send] = useQueueSend();
  const isSending = sending.status === "sending";
  const cleanTitle = cleanLine(title);
  const titleProblem = lengthProblem(cleanTitle, SESSION_TITLE);

  function choose(later: boolean) {
    setIsLater(later);
    setHasTried(false);
    setTimeProblem(null);
  }

  function changeTime(value: string) {
    setLaterValue(value);
    // Once a time was refused, say at once whether the new one fits.
    if (timeProblem !== null)
      setTimeProblem(scheduleProblem(value, serverNow()));
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSending) return;
    setHasTried(true);
    if (isLater) {
      const problem = scheduleProblem(laterValue, serverNow());
      const at = fromLocalInput(laterValue);
      setTimeProblem(problem);
      if (problem || !at) {
        document.getElementById(`${id}-at`)?.focus();
        return;
      }
      if (await send(() => setNextSession(project, at))) {
        setScheduled(at.toISOString());
      }
      return;
    }
    if (titleProblem) {
      document.getElementById(`${id}-title`)?.focus();
      return;
    }
    const session = { title: cleanTitle, plannedMinutes: minutes };
    if (await send(() => startSession(project, session))) onClose();
  }

  if (scheduled) {
    return (
      <Sheet title="Session scheduled" onClose={onClose}>
        <p className="sheet-lead">
          Planned for <strong>{whenLabel(scheduled)}</strong> (your time). The
          room&rsquo;s panel and its join page show it to everyone in their own
          time zone.
        </p>
        <AddToCalendar repo={project} startsAt={scheduled} />
        <div className="sheet-foot">
          <button type="button" className="btn primary" onClick={onClose}>
            Done
          </button>
        </div>
      </Sheet>
    );
  }

  const showTimeProblem = timeProblem !== null;
  return (
    <Sheet
      title={isLater ? "Schedule a session" : "Start a session"}
      onClose={onClose}
    >
      <p className="sheet-lead">
        {isLater
          ? "Everyone sees the time in their own time zone and can add it to their calendar."
          : "Everyone in the room gets an @all card and an unread badge."}
      </p>
      <form className="sheet-form" onSubmit={handleSubmit} noValidate>
        <fieldset className="field sheet-when">
          <legend>When</legend>
          <div className="sheet-when-options">
            <label className="sheet-check" htmlFor={`${id}-now`}>
              <input
                id={`${id}-now`}
                type="radio"
                name={`${id}-when`}
                checked={!isLater}
                onChange={() => choose(false)}
              />
              <span>Now</span>
            </label>
            <label className="sheet-check" htmlFor={`${id}-later`}>
              <input
                id={`${id}-later`}
                type="radio"
                name={`${id}-when`}
                checked={isLater}
                onChange={() => choose(true)}
              />
              <span>Later</span>
            </label>
          </div>
        </fieldset>
        {isLater ? (
          <div className="field">
            <label htmlFor={`${id}-at`}>
              Day and time <span className="sheet-optional">(your time)</span>
            </label>
            <input
              id={`${id}-at`}
              type="datetime-local"
              value={laterValue}
              onChange={(event) => changeTime(event.target.value)}
              aria-invalid={showTimeProblem}
              aria-describedby={
                showTimeProblem
                  ? `${id}-at-hint ${id}-at-problem`
                  : `${id}-at-hint`
              }
            />
            <p className="hint" id={`${id}-at-hint`}>
              You name the session and set its length when you go live.
            </p>
            {showTimeProblem && (
              <p className="err" id={`${id}-at-problem`}>
                {timeProblem}
              </p>
            )}
          </div>
        ) : (
          <>
            <TextField
              id={`${id}-title`}
              label="Title"
              value={title}
              onChange={setTitle}
              counter={{ count: charCount(cleanTitle), max: SESSION_TITLE.max }}
              problem={hasTried ? titleProblem : null}
            />
            <div className="field">
              <label htmlFor={`${id}-length`}>Planned length</label>
              <select
                id={`${id}-length`}
                value={minutes}
                onChange={(event) => setMinutes(Number(event.target.value))}
              >
                {LENGTHS.map((length) => (
                  <option key={length} value={length}>
                    {length === 0 ? "No set length" : `${length} min`}
                  </option>
                ))}
              </select>
            </div>
          </>
        )}
        <Refused sending={sending} />
        <div className="sheet-foot">
          <button type="button" className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={isSending}>
            {isLater
              ? isSending
                ? "Scheduling…"
                : "Schedule"
              : isSending
                ? "Going live…"
                : "Go live"}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
