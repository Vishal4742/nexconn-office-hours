import { nextHand } from "../../queue/api";
import { answeringHand, myOpenHand, waitingHands } from "../../queue/place";
import { actOnQueue } from "../../queue/store";
import type { Queue, QueueSession } from "../../queue/types";
import { HandIcon, NextIcon } from "./icons";
import { Said } from "./Said";
import { useAction } from "./useAction";

type Props = {
  readonly queue: Queue;
  readonly isMaintainer: boolean;
  readonly onStart: () => void;
  readonly onRaise: () => void;
};

/**
 * The one thing to do now, in the glass card at the bottom (docs/UI.md S3): Raise your hand for a
 * member, Next question for a maintainer, Start a session when none is live. When it can't be
 * done, the button says so and the line under it says why.
 */
export function MainAction({ queue, isMaintainer, onStart, onRaise }: Props) {
  const { session } = queue;
  if (!session?.live) {
    return isMaintainer ? (
      <div className="qcard q-foot">
        <button
          type="button"
          className="btn primary lg block"
          onClick={onStart}
        >
          Start a session
        </button>
      </div>
    ) : (
      <Disabled
        label="Raise your hand"
        note="Opens when the session is live."
      />
    );
  }
  if (isMaintainer) return <NextQuestion queue={queue} session={session} />;

  const mine = myOpenHand(queue);
  if (mine) {
    return (
      <Disabled
        label="Raise your hand"
        note={
          mine.status === "answering"
            ? "Your question is being answered now."
            : "You have one open hand."
        }
      />
    );
  }
  if (session.handsClosed) {
    return (
      <Disabled
        label="Hands are closed"
        note={`Last call: ${queue.hands.length} left to answer this session. Ask at the next one.`}
      />
    );
  }
  return (
    <div className="qcard q-foot">
      <button type="button" className="btn primary lg block" onClick={onRaise}>
        <HandIcon />
        Raise your hand
      </button>
    </div>
  );
}

function Disabled({
  label,
  note,
}: {
  readonly label: string;
  readonly note: string;
}) {
  return (
    <div className="qcard q-foot">
      <button type="button" className="btn primary lg block" disabled>
        <HandIcon />
        {label}
      </button>
      <p className="q-foot-note">{note}</p>
    </div>
  );
}

/** FR-QUEUE-3 (maintainer): takes the first waiting hand. One hand is answered at a time. */
function NextQuestion({
  queue,
  session,
}: {
  readonly queue: Queue;
  readonly session: QueueSession;
}) {
  const action = useAction();
  const current = answeringHand(queue);
  const count = waitingHands(queue).length;
  const reason = current
    ? `Finish or skip @${current.author.login} first.`
    : count === 0
      ? "Nobody is waiting."
      : "";
  const next = () =>
    void action.run(() => actOnQueue(() => nextHand(session.id)));

  return (
    <div className="qcard q-foot">
      <button
        type="button"
        className="btn primary lg block"
        disabled={reason !== "" || action.isBusy}
        onClick={next}
      >
        <NextIcon />
        Next question
        {count > 0 && (
          <span className="q-badge">
            {count}
            <span className="sr-only"> waiting</span>
          </span>
        )}
      </button>
      {reason && <p className="q-foot-note">{reason}</p>}
      <Said doing={action.doing} />
    </div>
  );
}
