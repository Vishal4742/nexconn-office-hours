// NFR-A11Y-1: the sentence a screen reader hears, politely, when the queue changes. It speaks
// only of what someone in the room needs: the session, who is being answered, what became of my
// hand and where it is, and how many wait. "It's your turn" is the your-turn banner's, said at
// once (aria-live="assertive"), so it is not said here a second time.
import {
  aheadOfMe,
  answeringHand,
  myOpenHand,
  placeLabel,
  waitingHands,
} from "./place";
import type { HandStatus, Queue } from "./types";

/** What became of my hand when it left the queue. */
const MY_FATE: Readonly<Partial<Record<HandStatus, string>>> = {
  answered: "Your question was answered.",
  withdrawn: "You withdrew your hand.",
  skipped: "Your hand was skipped.",
  removed: "A maintainer removed your hand.",
  merged: "Your question joined the one being answered.",
  carried: "Your question moves to the front of the next session.",
  not_reached: "Your question was not reached this time.",
};

function sessionNews(before: Queue, after: Queue): string {
  const was = before.session;
  const is = after.session;
  if (!is) return "";
  const isSame = was?.id === is.id;
  if (is.live && !(isSame && was?.live)) {
    return `The session started: ${is.title}.`;
  }
  if (!isSame || !was?.live) return "";
  if (!is.live) return "The session ended.";
  if (is.handsClosed === was.handsClosed) return "";
  return is.handsClosed ? "Hands are closed." : "Hands are open again.";
}

function answeringNews(before: Queue, after: Queue): string {
  const current = answeringHand(after);
  if (!current || current.isMine) return "";
  return current.id === answeringHand(before)?.id
    ? ""
    : `@${current.author.login} is being answered.`;
}

function myHandNews(before: Queue, after: Queue): string {
  const mine = myOpenHand(before);
  if (!mine) return "";
  const now = [...after.hands, ...after.done].find(
    (hand) => hand.id === mine.id,
  );
  if (!now) return "";
  if (mine.status === "answering" && now.status === "waiting") {
    return "You missed your turn, so you moved back.";
  }
  return MY_FATE[now.status] ?? "";
}

function placeNews(before: Queue, after: Queue): string {
  const ahead = aheadOfMe(after);
  return ahead === null || ahead === aheadOfMe(before)
    ? ""
    : `${placeLabel(ahead)}.`;
}

/** How many wait, for someone without an open hand: their place says the rest. */
function countNews(before: Queue, after: Queue): string {
  if (!after.session?.live || myOpenHand(after)) return "";
  const count = waitingHands(after).length;
  if (count === waitingHands(before).length) return "";
  return count === 0 ? "Nobody is waiting." : `${count} waiting.`;
}

/** What changed from one queue to the next, in a sentence or two; "" when nothing worth saying. */
export function queueNews(before: Queue | null, after: Queue): string {
  if (!before || before === after) return "";
  return [sessionNews, answeringNews, myHandNews, placeNews, countNews]
    .map((news) => news(before, after))
    .filter((sentence) => sentence !== "")
    .join(" ");
}
