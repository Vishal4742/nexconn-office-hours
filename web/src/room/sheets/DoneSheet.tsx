import { useId, useState } from "react";
import type { FormEvent } from "react";

import { Sheet } from "../../components/Sheet";
import type { Project } from "../../projects";
import { finishHand } from "../../queue/api";
import type { Hand } from "../../queue/types";
import {
  ANSWER,
  MAX_LINKS,
  charCount,
  cleanText,
  lengthProblem,
  readLinks,
} from "./limits";
import { Refused, TextField, useQueueSend } from "./parts";

type Props = {
  readonly project: Project;
  /** The hand being answered. */
  readonly hand: Hand;
  /**
   * Whether the hand is still the one being answered. A co-maintainer or another tab may skip or
   * finish it meanwhile: the sheet then keeps what was typed, to copy, and can't publish it
   * (state-machine audit, bug 8).
   */
  readonly isStillAnswering: boolean;
  readonly onClose: () => void;
};

function askedBy(hand: Hand): string {
  const others =
    hand.plusCount === 0
      ? ""
      : hand.plusCount === 1
        ? " and 1 other person"
        : ` and ${hand.plusCount} other people`;
  return `Asked by @${hand.author.login}${others}.`;
}

/**
 * A maintainer finishes the hand being answered: an answer summary and optional links, which
 * become a FAQ entry (FR-FAQ-1). The props are a contract (notes/phase-2-plan.md).
 */
export function DoneSheet({ hand, isStillAnswering, onClose }: Props) {
  const id = useId();
  const [answer, setAnswer] = useState("");
  const [linkText, setLinkText] = useState("");
  const [hasTried, setHasTried] = useState(false);
  const [sending, send] = useQueueSend();
  const isSending = sending.status === "sending";
  const cleanAnswer = cleanText(answer);
  const answerProblem = lengthProblem(cleanAnswer, ANSWER);
  const { links, problem: linksProblem } = readLinks(linkText);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSending || !isStillAnswering) return;
    setHasTried(true);
    const wrong = answerProblem ? "answer" : linksProblem ? "links" : null;
    if (wrong) {
      document.getElementById(`${id}-${wrong}`)?.focus();
      return;
    }
    const entry = { answer: cleanAnswer, links };
    if (await send(() => finishHand(hand.id, entry))) onClose();
  }

  return (
    <Sheet title="Publish the answer" onClose={onClose}>
      <p className="sheet-lead">
        {askedBy(hand)} The answer becomes a public FAQ page and a card in the
        room.
      </p>
      <div className="sheet-asked">
        <p className="sheet-eyebrow">The question</p>
        <p className="sheet-asked-title">{hand.title}</p>
        {hand.details && <p className="sheet-asked-details">{hand.details}</p>}
      </div>
      <form className="sheet-form" onSubmit={handleSubmit} noValidate>
        <TextField
          id={`${id}-answer`}
          label="Answer summary"
          rows={5}
          value={answer}
          onChange={setAnswer}
          placeholder="Two or three sentences someone can act on without the chat."
          counter={{ count: charCount(cleanAnswer), max: ANSWER.max }}
          hint={`At least ${ANSWER.min} characters.`}
          problem={hasTried ? answerProblem : null}
        />
        <TextField
          id={`${id}-links`}
          label="Links"
          isOptional
          rows={2}
          value={linkText}
          onChange={setLinkText}
          placeholder="https://"
          hint={`Up to ${MAX_LINKS}, one per line, each starting with http:// or https://.`}
          problem={hasTried ? linksProblem : null}
        />
        <div className="sheet-preview">
          <p className="sheet-eyebrow">Preview</p>
          <p className="sheet-preview-title">{hand.title}</p>
          {cleanAnswer ? (
            <p className="sheet-preview-answer">{cleanAnswer}</p>
          ) : (
            <p className="sheet-preview-answer muted">
              The answer summary shows here.
            </p>
          )}
          {links.length > 0 && (
            <ul className="sheet-preview-links">
              {links.map((link) => (
                <li key={link}>{link}</li>
              ))}
            </ul>
          )}
        </div>
        <Refused sending={sending} />
        {!isStillAnswering && (
          <p className="err" role="alert">
            This question isn&rsquo;t being answered any more: someone skipped
            or finished it. Copy your answer if you want to keep it.
          </p>
        )}
        <div className="sheet-foot">
          <button type="button" className="btn ghost" onClick={onClose}>
            {isStillAnswering ? "Cancel" : "Close"}
          </button>
          <button
            type="submit"
            className="btn primary"
            disabled={isSending || !isStillAnswering}
          >
            {isSending ? "Publishing…" : "Publish to FAQ"}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
