import { useId, useState } from "react";

import { Sheet } from "../../components/Sheet";
import { endSession } from "../../queue/api";
import type { Hand, QueueSession } from "../../queue/types";
import { Refused, useQueueSend } from "./parts";

type Props = {
  readonly session: QueueSession;
  /** How many hands are still waiting: what the carry-over choice is about. */
  readonly waitingCount: number;
  /**
   * The hand being answered, if any. The server refuses to end a session in the middle of an
   * answer, so the sheet says so instead of offering End (state-machine audit, bug 7).
   */
  readonly answering: Hand | null;
  readonly onClose: () => void;
};

function stillWaiting(count: number): string {
  if (count === 0) return "Nobody is waiting.";
  return count === 1
    ? "1 question is still waiting."
    : `${count} questions are still waiting.`;
}

function carryLabel(count: number): string {
  return count === 1
    ? "Carry the waiting question over to the next session"
    : `Carry the ${count} waiting questions over to the next session`;
}

/**
 * "End this session?" with the carry-over choice, ticked by default and left out when nobody
 * waits (FR-SESS-4, FR-SESS-7). The props are a contract (notes/phase-2-plan.md).
 */
export function EndSessionSheet({
  session,
  waitingCount,
  answering,
  onClose,
}: Props) {
  const carryId = useId();
  const [carry, setCarry] = useState(true);
  const [sending, send] = useQueueSend();
  const isSending = sending.status === "sending";

  async function handleEnd() {
    if (isSending || answering) return;
    // With nobody waiting there is nothing to carry; the server's default is kept.
    const isCarried = waitingCount === 0 || carry;
    if (await send(() => endSession(session.id, isCarried))) onClose();
  }

  return (
    <Sheet title="End this session?" onClose={onClose}>
      {answering ? (
        <p className="sheet-lead">
          @{answering.author.login}&rsquo;s question is being answered. Finish
          or skip it first: a session can&rsquo;t end in the middle of an
          answer.
        </p>
      ) : (
        <p className="sheet-lead">
          {stillWaiting(waitingCount)} The room gets a summary card.
        </p>
      )}
      {waitingCount > 0 && !answering && (
        <div className="field">
          <label className="sheet-check" htmlFor={carryId}>
            <input
              id={carryId}
              type="checkbox"
              checked={carry}
              onChange={(event) => setCarry(event.target.checked)}
              aria-describedby={`${carryId}-hint`}
            />
            <span>{carryLabel(waitingCount)}</span>
          </label>
          <p className="hint" id={`${carryId}-hint`}>
            They go to the front of the line, in their order. Unticked, they are
            marked as not reached.
          </p>
        </div>
      )}
      <Refused sending={sending} />
      <div className="sheet-foot">
        <button type="button" className="btn ghost" onClick={onClose}>
          Keep going
        </button>
        <button
          type="button"
          className="btn sheet-ink"
          disabled={isSending || answering !== null}
          onClick={handleEnd}
        >
          {isSending ? "Ending…" : "End session"}
        </button>
      </div>
    </Sheet>
  );
}
