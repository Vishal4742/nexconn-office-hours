// FR-NOTIF-2: what changed for me between the queue the page held and the one that just came.
// Plain functions of the two queues, like place.ts; room/turnNotice.ts says it out loud.
import { aheadOfMe, myOpenHand } from "./place";
import type { Hand, Queue } from "./types";

export type TurnAlert = {
  /** "next": my hand just became first in line. "turn": it just started being answered. */
  readonly kind: "next" | "turn";
  readonly hand: Hand;
};

/**
 * My hand just started being answered, or just became first in line, including a new session that
 * starts with my carried hand first. Null on the first queue a page gets (the banner and the tab
 * title already say where I am), when nothing changed for me, and when the change is not mine.
 */
export function turnAlert(
  before: Queue | null,
  after: Queue,
): TurnAlert | null {
  if (!before) return null;
  const mine = myOpenHand(after);
  if (!mine) return null;
  const mineBefore = myOpenHand(before);
  const isSameHand = mineBefore?.id === mine.id;
  if (mine.status === "answering") {
    const wasAnswering = isSameHand && mineBefore.status === "answering";
    return wasAnswering ? null : { kind: "turn", hand: mine };
  }
  if (aheadOfMe(after) !== 0) return null;
  const wasNext = isSameHand && aheadOfMe(before) === 0;
  return wasNext ? null : { kind: "next", hand: mine };
}
