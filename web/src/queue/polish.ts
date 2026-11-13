// Small things the room says on top of the queue: my place in the tab's title, how long I may
// wait, and how long the question being answered has taken. Plain logic, so it can be tested.
import { toMs } from "./clock";
import { aheadOfMe, answeringHand, myOpenHand } from "./place";
import type { Queue } from "./types";

/**
 * FR-QUEUE-11: the tab's title carries my place, so a person in another tab sees it move:
 * "(2 ahead) …", "(You're next) …", "(Your turn) …". The plain title without a hand of mine.
 */
export function tabTitle(base: string, queue: Queue): string {
  if (!queue.session?.live) return base;
  const mine = myOpenHand(queue);
  if (mine?.status === "answering") return `(Your turn) ${base}`;
  const ahead = aheadOfMe(queue);
  if (ahead === null) return base;
  return ahead === 0 ? `(You're next) ${base}` : `(${ahead} ahead) ${base}`;
}

/**
 * FR-QUEUE-10: about how long until my turn, from this session's median answer time: the hands
 * ahead of mine, and the one being answered now. Nothing until two answers give a median, nothing
 * when nobody is before me, and never less than a minute.
 */
export function waitLabel(
  ahead: number,
  medianSeconds: number,
  isSomeoneAnswered: boolean,
): string {
  if (medianSeconds <= 0) return "";
  const turns = ahead + (isSomeoneAnswered ? 1 : 0);
  if (turns === 0) return ""; // next, and nobody before me: no wait to tell
  const minutes = Math.max(1, Math.round((turns * medianSeconds) / 60));
  return `about ${minutes} min`;
}

/** waitLabel for a queue as it is. */
export function waitFor(queue: Queue): string {
  const ahead = aheadOfMe(queue);
  if (ahead === null) return "";
  return waitLabel(
    ahead,
    queue.medianAnswerSeconds,
    answeringHand(queue) !== null,
  );
}

/** The maintainer's timer on the question being answered (FR-QUEUE-12). */
export type QuestionTimer = {
  readonly label: string;
  readonly isLong: boolean;
};

/** After this long a question is long: the timer turns amber. */
const LONG_MS = 8 * 60_000;

/** "4:07" since the hand was taken, long from 8 minutes on; "" when the time can't be read. */
export function questionTimer(
  takenAt: string | null,
  serverNow: number,
): QuestionTimer {
  const since = takenAt ? serverNow - toMs(takenAt) : Number.NaN;
  if (Number.isNaN(since)) return { label: "", isLong: false };
  const seconds = Math.max(0, Math.floor(since / 1000));
  const label = `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
  return { label, isLong: since >= LONG_MS };
}
