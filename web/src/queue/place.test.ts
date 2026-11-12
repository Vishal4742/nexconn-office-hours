import { describe, expect, it } from "vitest";

import { ana, diego, hand, liveSession, liwen, mia, queues } from "./fixtures";
import {
  FATE,
  aheadOfMe,
  answeringHand,
  myOpenHand,
  placeLabel,
  summarize,
  waitingHands,
} from "./place";
import type { Queue, QueuePerson } from "./types";

const person = (n: number): QueuePerson => ({
  userId: `gh${100 + n}`,
  login: `member-${n}`,
  name: `Member ${n}`,
  avatarUrl: "",
});

/** A live queue of `length` waiting hands, mine at `place` (counted from 0). */
function lineOf(length: number, place: number, isAnswering: boolean): Queue {
  const waiting = Array.from({ length }, (_, at) =>
    hand(String(200 + at), at === place ? diego : person(at), {
      isMine: at === place,
    }),
  );
  const current = hand("199", mia, { status: "answering", takenBy: mia });
  return {
    ...queues.liveEmpty,
    session: liveSession,
    hands: isAnswering ? [current, ...waiting] : waiting,
  };
}

describe("who is where in the queue", () => {
  it("finds the hand being answered, if there is one", () => {
    expect(answeringHand(queues.liveWatching)?.id).toBe("101");
    expect(answeringHand(queues.lastCall)).toBeNull();
    expect(answeringHand(queues.noSession)).toBeNull();
  });

  it("FR-QUEUE-1: lists the waiting hands in the server's order", () => {
    expect(waitingHands(queues.liveWaiting).map((each) => each.id)).toEqual([
      "102",
      "103",
      "104",
    ]);
    expect(waitingHands(queues.liveEmpty)).toEqual([]);
  });

  it("finds my open hand, waiting or being answered", () => {
    expect(myOpenHand(queues.liveWaiting)?.id).toBe("104");
    expect(myOpenHand(queues.myTurn)?.status).toBe("answering");
    expect(myOpenHand(queues.liveWatching)).toBeNull();
    expect(myOpenHand(queues.ended)).toBeNull();
  });
});

describe("aheadOfMe", () => {
  it("FR-QUEUE-5: counts the hands that wait before mine, never the one being answered", () => {
    expect(aheadOfMe(queues.liveWaiting)).toBe(2);
    expect(aheadOfMe(queues.liveNext)).toBe(0);
  });

  it("FR-QUEUE-5: is right for every place in a line of five, with and without a hand being answered", () => {
    for (const isAnswering of [true, false]) {
      for (let place = 0; place < 5; place++) {
        expect(aheadOfMe(lineOf(5, place, isAnswering))).toBe(place);
      }
    }
  });

  it("is right when mine is the only hand", () => {
    expect(aheadOfMe(lineOf(1, 0, false))).toBe(0);
    expect(aheadOfMe(lineOf(1, 0, true))).toBe(0);
  });

  it("is nothing without a waiting hand of mine", () => {
    expect(aheadOfMe(queues.liveWatching)).toBeNull();
    expect(aheadOfMe(queues.liveEmpty)).toBeNull();
    expect(aheadOfMe(queues.noSession)).toBeNull();
    expect(aheadOfMe(queues.ended)).toBeNull();
  });

  it("is nothing while my hand is the one being answered", () => {
    expect(aheadOfMe(queues.myTurn)).toBeNull();
  });
});

describe("placeLabel", () => {
  it("FR-QUEUE-5: says how many are ahead, or that I am next", () => {
    expect(placeLabel(0)).toBe("You're next");
    expect(placeLabel(1)).toBe("1 ahead of you");
    expect(placeLabel(2)).toBe("2 ahead of you");
    expect(placeLabel(14)).toBe("14 ahead of you");
  });
});

describe("summarize", () => {
  it("FR-SESS-4, FR-SESS-7: counts what became of an ended session's questions", () => {
    expect(summarize(queues.ended)).toEqual({
      questions: 3,
      answered: 1,
      carried: 1,
      notReached: 1,
    });
  });

  it("FR-MOD-1: a hand a maintainer removed is not counted as a question", () => {
    const onlyRemoved: Queue = {
      ...queues.ended,
      done: [hand("105", diego, { status: "removed", title: "" })],
    };
    expect(summarize(onlyRemoved).questions).toBe(0);
  });

  it("counts every other ending as a question that was asked", () => {
    const mixed: Queue = {
      ...queues.ended,
      done: [
        hand("1", ana, { status: "answered" }),
        hand("2", liwen, { status: "answered" }),
        hand("3", diego, { status: "skipped" }),
        hand("4", mia, { status: "withdrawn" }),
        hand("5", person(1), { status: "merged" }),
        hand("6", person(2), { status: "not_reached" }),
      ],
    };
    expect(summarize(mixed)).toEqual({
      questions: 6,
      answered: 2,
      carried: 0,
      notReached: 1,
    });
  });

  it("is all zeros for a session nobody asked in", () => {
    expect(summarize({ ...queues.ended, done: [] })).toEqual({
      questions: 0,
      answered: 0,
      carried: 0,
      notReached: 0,
    });
  });
});

describe("FATE", () => {
  it("has a word for what became of a hand, whatever its status", () => {
    expect(FATE.answered).toBe("Answered");
    expect(FATE.carried).toBe("Next session");
    expect(FATE.not_reached).toBe("Not reached");
    expect(FATE.removed).toBe("Removed");
    for (const word of Object.values(FATE)) expect(word).not.toBe("");
  });
});
