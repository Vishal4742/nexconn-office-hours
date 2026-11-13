import { useState } from "react";

import type { Project } from "../../projects";
import {
  aheadOfMe,
  answeringHand,
  myOpenHand,
  waitingHands,
} from "../../queue/place";
import { waitFor } from "../../queue/polish";
import type { Hand, Queue, QueueSession } from "../../queue/types";
import { DoneList } from "./DoneList";
import { MyPlace } from "./MyPlace";
import { NowCard } from "./NowCard";
import { WaitingCard } from "./WaitingCard";

type Props = {
  readonly project: Project;
  /** A queue whose session is live. */
  readonly queue: Queue;
  readonly session: QueueSession;
  readonly isMaintainer: boolean;
  readonly onDone: (hand: Hand) => void;
};

/**
 * The cards of a live session, top to bottom: who is being answered, where I am, who waits, and
 * what was done (docs/UI.md S3). The order of the hands is the server's (FR-QUEUE-1).
 */
export function LiveQueue({
  project,
  queue,
  session,
  isMaintainer,
  onDone,
}: Props) {
  const current = answeringHand(queue);
  const waiting = waitingHands(queue);
  const mine = myOpenHand(queue);
  const ahead = aheadOfMe(queue);
  // The list sits in a different card when nobody waits, so it is drawn anew; its open state
  // lives here so that it stays as the reader left it.
  const [isDoneOpen, setDoneOpen] = useState(false);
  const done = (
    <DoneList
      project={project}
      done={queue.done}
      isMaintainer={isMaintainer}
      isOpen={isDoneOpen}
      onToggle={setDoneOpen}
    />
  );

  return (
    <>
      {current ? (
        <NowCard
          project={project}
          hand={current}
          isMaintainer={isMaintainer}
          onDone={onDone}
        />
      ) : (
        waiting.length > 0 && (
          <p className="qcard q-note">
            {isMaintainer
              ? "Press Next question to take the first hand."
              : `${session.host ? `@${session.host.login}` : "A maintainer"} will take the first hand in a moment.`}
          </p>
        )
      )}
      {mine && ahead !== null && (
        <MyPlace hand={mine} ahead={ahead} wait={waitFor(queue)} />
      )}
      {waiting.length > 0 ? (
        <WaitingCard
          project={project}
          waiting={waiting}
          isMaintainer={isMaintainer}
          canMerge={current !== null}
          closedLeft={session.handsClosed ? queue.hands.length : null}
        >
          {done}
        </WaitingCard>
      ) : (
        <section className="qcard q-wait q-wait-slim" aria-label="Waiting">
          <p className="q-note-title">
            {current
              ? "Nobody else is waiting"
              : queue.done.length > 0
                ? "Nobody is waiting"
                : "No hands yet"}
          </p>
          <p className="muted">
            {session.handsClosed
              ? "Hands are closed for this session."
              : "Questions show up here in the order they're raised."}
          </p>
          {done}
        </section>
      )}
    </>
  );
}
