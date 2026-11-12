import { useState } from "react";
import type { FormEvent } from "react";

import { CalendarIcon } from "../../components/Icons";
import type { Project } from "../../projects";
import { setNextSession } from "../../queue/api";
import { summarize } from "../../queue/place";
import type { Summary } from "../../queue/place";
import { actOnQueue } from "../../queue/store";
import { fromLocalInput, toLocalInput, whenLabel } from "../../queue/times";
import type { Queue } from "../../queue/types";
import { DoneList } from "./DoneList";
import { StarIcon } from "./icons";
import { Said } from "./Said";
import { useAction } from "./useAction";

type Props = {
  readonly project: Project;
  /** A queue whose session ended, or that never had one. */
  readonly queue: Queue;
  readonly isMaintainer: boolean;
};

const plural = (count: number, word: string) =>
  `${count} ${word}${count === 1 ? "" : "s"}`;

/** FR-SESS-4, FR-SESS-7: "12 questions · 9 answered · 1 carried over · 2 not reached". */
function summaryLine(summary: Summary): string {
  return [
    plural(summary.questions, "question"),
    `${summary.answered} answered`,
    summary.carried > 0 ? `${summary.carried} carried over` : "",
    summary.notReached > 0 ? `${summary.notReached} not reached` : "",
  ]
    .filter((part) => part !== "")
    .join(" · ");
}

/**
 * The panel while no session is live: "No session right now", or the summary of the one that
 * ended, and the next session's time in the viewer's time zone (FR-SESS-5), which a maintainer
 * sets and clears here.
 */
export function OffQueue({ project, queue, isMaintainer }: Props) {
  const { session, nextSessionAt } = queue;
  const isMineCarried = queue.done.some(
    (hand) => hand.isMine && hand.status === "carried",
  );

  return (
    <>
      <section
        className="qcard off-card"
        aria-label={session ? "Session ended" : "No session"}
      >
        <CalendarIcon />
        <div className="q-off-text">
          <p className="off-title">
            {session ? "Session ended" : "No session right now"}
          </p>
          <p className="muted">
            {session
              ? summaryLine(summarize(queue))
              : "When a session is live, the queue of questions shows here."}
          </p>
          {nextSessionAt && (
            <p className="q-next-at">
              Next session:{" "}
              <span className="q-mono">{whenLabel(nextSessionAt)}</span> (your
              time)
            </p>
          )}
          {isMaintainer && (
            <NextSessionForm
              key={nextSessionAt ?? "none"}
              project={project}
              at={nextSessionAt}
            />
          )}
        </div>
      </section>
      {isMineCarried && (
        <section className="q-mine" aria-label="Your question">
          <p className="q-mine-pos">
            <StarIcon />
            Your question is saved
          </p>
          <p className="q-mine-note">
            It moves to the front of the next session. You don&rsquo;t need to
            ask again.
          </p>
        </section>
      )}
      {queue.done.length > 0 && (
        <section className="qcard q-wait q-wait-slim" aria-label="This session">
          <DoneList
            project={project}
            done={queue.done}
            isMaintainer={isMaintainer}
            isOpen
          />
        </section>
      )}
    </>
  );
}

type FormProps = {
  readonly project: Project;
  readonly at: string | null;
};

/** FR-SESS-5 (maintainer): a native date and time field, on the viewer's own clock. */
function NextSessionForm({ project, at }: FormProps) {
  const [value, setValue] = useState(at ? toLocalInput(at) : "");
  const action = useAction();

  function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const when = fromLocalInput(value);
    void action.run(async () => {
      if (!when) throw new Error("Pick a day and a time first.");
      await actOnQueue(() => setNextSession(project, when));
    });
  }

  const clear = () =>
    void action.run(() => actOnQueue(() => setNextSession(project, null)));

  return (
    <form className="q-next" onSubmit={handleSave} noValidate>
      <label htmlFor="q-next-at">
        {at ? "Move the next session" : "Plan the next session"}
      </label>
      <div className="q-btns">
        <input
          id="q-next-at"
          type="datetime-local"
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
        <button type="submit" className="btn ghost sm" disabled={action.isBusy}>
          Save
        </button>
        {at && (
          <button
            type="button"
            className="btn ghost sm"
            disabled={action.isBusy}
            onClick={clear}
          >
            Clear
          </button>
        )}
      </div>
      <Said doing={action.doing} />
    </form>
  );
}
