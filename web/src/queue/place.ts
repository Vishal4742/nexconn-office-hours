// Who is where in a queue, as the panel and its announcements say it. Plain functions of the
// queue the server sent: the order is the server's (FR-QUEUE-1), never re-sorted here.
import type { Hand, HandStatus, Queue } from "./types";

/** The hand being answered, if there is one. */
export function answeringHand(queue: Queue): Hand | null {
  return queue.hands.find((hand) => hand.status === "answering") ?? null;
}

/** The waiting hands, in the server's order. */
export function waitingHands(queue: Queue): readonly Hand[] {
  return queue.hands.filter((hand) => hand.status === "waiting");
}

/** My open hand, waiting or being answered (FR-HAND-2: there is at most one). */
export function myOpenHand(queue: Queue): Hand | null {
  return queue.hands.find((hand) => hand.isMine) ?? null;
}

/**
 * FR-QUEUE-5: how many waiting hands come before mine. The hand being answered is never counted.
 * Null when I have no waiting hand.
 */
export function aheadOfMe(queue: Queue): number | null {
  const place = waitingHands(queue).findIndex((hand) => hand.isMine);
  return place === -1 ? null : place;
}

/** FR-QUEUE-5: "2 ahead of you", or "You're next". */
export function placeLabel(ahead: number): string {
  return ahead === 0 ? "You're next" : `${ahead} ahead of you`;
}

export type Summary = {
  readonly questions: number;
  readonly answered: number;
  readonly carried: number;
  readonly notReached: number;
};

/**
 * FR-SESS-4, FR-SESS-7: what became of an ended session's questions. A hand a maintainer removed
 * is not counted as a question (FR-MOD-1).
 */
export function summarize(queue: Queue): Summary {
  const asked = [...queue.hands, ...queue.done].filter(
    (hand) => hand.status !== "removed",
  );
  const count = (status: HandStatus) =>
    asked.filter((hand) => hand.status === status).length;
  return {
    questions: asked.length,
    answered: count("answered"),
    carried: count("carried"),
    notReached: count("not_reached"),
  };
}

/** What became of a hand, in a word or two (docs/UI.md, the status tokens). */
export const FATE: Readonly<Record<HandStatus, string>> = {
  waiting: "Waiting",
  answering: "Being answered",
  answered: "Answered",
  skipped: "Skipped",
  withdrawn: "Withdrawn",
  removed: "Removed",
  not_reached: "Not reached",
  carried: "Next session",
  merged: "Merged",
};
