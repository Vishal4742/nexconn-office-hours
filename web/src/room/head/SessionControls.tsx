import { setLastCall } from "../../queue/api";
import type { QueueSession } from "../../queue/types";
import { Refused, useQueueSend } from "../sheets/parts";

type Props = {
  /** The live session, else the last one that ended, else null. */
  readonly session: QueueSession | null;
  readonly onStart: () => void;
  readonly onEnd: () => void;
};

function lastCallText(isClosed: boolean, isSending: boolean): string {
  if (isSending) return isClosed ? "Opening hands…" : "Closing hands…";
  return isClosed ? "Open hands" : "Last call";
}

/**
 * A maintainer's controls in the room's head: while a session is live, Last call or Open hands
 * (FR-SESS-6) and End session (its sheet asks first, FR-SESS-4); otherwise Start a session.
 */
export function SessionControls({ session, onStart, onEnd }: Props) {
  const [sending, send] = useQueueSend();

  if (!session?.live) {
    return (
      <button type="button" className="btn primary sm" onClick={onStart}>
        Start a session
      </button>
    );
  }

  const { id, handsClosed } = session;
  const isSending = sending.status === "sending";
  return (
    <>
      <div className="rh-actions">
        <button
          type="button"
          className="btn ghost sm"
          disabled={isSending}
          onClick={() => void send(() => setLastCall(id, !handsClosed))}
        >
          {lastCallText(handsClosed, isSending)}
        </button>
        <button type="button" className="btn ghost sm" onClick={onEnd}>
          End session
        </button>
      </div>
      <Refused sending={sending} />
    </>
  );
}
