import { useId, useState } from "react";
import type { FormEvent } from "react";

import { Sheet } from "../../components/Sheet";
import type { Project } from "../../projects";
import { startSession } from "../../queue/api";
import { SESSION_TITLE, charCount, cleanLine, lengthProblem } from "./limits";
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
 * A maintainer starts a session: a title and an optional length (FR-SESS-1). On success it gives
 * the answer to acceptQueue and closes. The props are a contract (notes/phase-2-plan.md).
 */
export function StartSessionSheet({ project, onClose }: Props) {
  const id = useId();
  const [title, setTitle] = useState(usualTitle);
  const [minutes, setMinutes] = useState<number>(USUAL_LENGTH);
  const [hasTried, setHasTried] = useState(false);
  const [sending, send] = useQueueSend();
  const isSending = sending.status === "sending";
  const cleanTitle = cleanLine(title);
  const titleProblem = lengthProblem(cleanTitle, SESSION_TITLE);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSending) return;
    setHasTried(true);
    if (titleProblem) {
      document.getElementById(`${id}-title`)?.focus();
      return;
    }
    const session = { title: cleanTitle, plannedMinutes: minutes };
    if (await send(() => startSession(project, session))) onClose();
  }

  return (
    <Sheet title="Start a session" onClose={onClose}>
      <p className="sheet-lead">
        Everyone in the room gets an @all card and an unread badge.
      </p>
      <form className="sheet-form" onSubmit={handleSubmit} noValidate>
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
        <Refused sending={sending} />
        <div className="sheet-foot">
          <button type="button" className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={isSending}>
            {isSending ? "Going live…" : "Go live"}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
