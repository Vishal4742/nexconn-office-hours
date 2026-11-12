import type { Queue } from "../queue/types";

type Props = {
  /** Null until the queue has loaded. */
  readonly queue: Queue | null;
};

/**
 * "It's your turn" (FR-QUEUE-5, FR-QUEUE-9): shown while my hand is the one being answered, with
 * the "I'm here" button. It follows the queue the server returned, never a message alone: cards
 * and pings are replayed after a reconnect. The props are a contract (notes/phase-2-plan.md); the
 * banner itself is the sheets-and-head part of the build (worker 6).
 */
export function YourTurnBanner({ queue }: Props) {
  const isMyTurn =
    queue?.hands.some((hand) => hand.isMine && hand.status === "answering") ??
    false;
  if (!isMyTurn) return null;
  return (
    <p className="sr-only" role="alert">
      It's your turn.
    </p>
  );
}
