import { useId, useState } from "react";
import type { FormEvent } from "react";

import { Sheet } from "../../components/Sheet";
import type { Project } from "../../projects";
import { raiseHand } from "../../queue/api";
import type { QueueSession } from "../../queue/types";
import {
  HAND_DETAILS,
  HAND_TITLE,
  charCount,
  cleanLine,
  cleanText,
  lengthProblem,
} from "./limits";
import { Refused, TextField, useQueueSend } from "./parts";
import { SimilarAnswers } from "./SimilarAnswers";

type Props = {
  readonly project: Project;
  /** The live session the hand is raised in. */
  readonly session: QueueSession;
  readonly onClose: () => void;
};

/**
 * A member raises a hand: a title and optional details, with up to 3 similar FAQ entries shown
 * while they type (FR-HAND-1, FR-HAND-3). The props are a contract (notes/phase-2-plan.md).
 */
export function RaiseHandSheet({ project, session, onClose }: Props) {
  const id = useId();
  const [title, setTitle] = useState("");
  const [details, setDetails] = useState("");
  const [hasTried, setHasTried] = useState(false);
  const [sending, send] = useQueueSend();
  const isSending = sending.status === "sending";
  const cleanTitle = cleanLine(title);
  const cleanDetails = cleanText(details);
  const titleProblem = lengthProblem(cleanTitle, HAND_TITLE);
  const detailsProblem = lengthProblem(cleanDetails, HAND_DETAILS);
  const host = session.host ? `@${session.host.login}` : "The maintainer";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSending) return;
    setHasTried(true);
    const wrong = titleProblem ? "title" : detailsProblem ? "details" : null;
    if (wrong) {
      document.getElementById(`${id}-${wrong}`)?.focus();
      return;
    }
    const hand = { title: cleanTitle, details: cleanDetails };
    if (await send(() => raiseHand(session.id, hand))) onClose();
  }

  return (
    <Sheet title="Raise your hand" onClose={onClose}>
      <p className="sheet-lead">
        {host} takes questions in order. Write yours so the next person can find
        it too.
      </p>
      <form className="sheet-form" onSubmit={handleSubmit} noValidate>
        <TextField
          id={`${id}-title`}
          label="Your question"
          value={title}
          onChange={setTitle}
          placeholder="For example: how do I retry only on 5xx errors?"
          counter={{ count: charCount(cleanTitle), max: HAND_TITLE.max }}
          problem={hasTried ? titleProblem : null}
        />
        <SimilarAnswers
          project={project}
          title={title}
          details={details}
          onSolved={onClose}
        />
        <TextField
          id={`${id}-details`}
          label="Details"
          isOptional
          rows={4}
          value={details}
          onChange={setDetails}
          placeholder="What you tried, what happened, which versions."
          counter={{ count: charCount(cleanDetails), max: HAND_DETAILS.max }}
          problem={hasTried ? detailsProblem : null}
        />
        <Refused sending={sending} />
        <div className="sheet-foot">
          <button type="button" className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={isSending}>
            {isSending ? "Raising your hand…" : "Raise my hand"}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
