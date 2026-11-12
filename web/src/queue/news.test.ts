import { describe, expect, it } from "vitest";

import { ana, diego, hand, liveSession, liwen, mia, queues } from "./fixtures";
import { queueNews } from "./news";
import type { Hand, HandStatus, Queue } from "./types";

/** The live session with these hands. */
const live = (hands: readonly Hand[], done: readonly Hand[] = []): Queue => ({
  ...queues.liveEmpty,
  hands,
  done,
});
const answering = (id: string, author = ana, over: Partial<Hand> = {}) =>
  hand(id, author, { status: "answering", takenBy: mia, ...over });
const closed = (id: string, author: typeof ana, status: HandStatus) =>
  hand(id, author, { status, closedAt: "2026-11-14T11:49:00.000Z" });
const mine = (id: string, over: Partial<Hand> = {}) =>
  hand(id, diego, { isMine: true, ...over });

describe("queueNews: NFR-A11Y-1, what the polite line says", () => {
  it("says nothing on the first load: the panel itself is there to be read", () => {
    expect(queueNews(null, queues.liveWatching)).toBe("");
  });

  it("says nothing when nothing it speaks of changed", () => {
    expect(queueNews(queues.liveWatching, queues.liveWatching)).toBe("");
    const [current, first, second] = queues.liveWatching.hands;
    const plussed = live([current, { ...first, plusCount: 3 }, second]);
    expect(queueNews(queues.liveWatching, plussed)).toBe("");
  });

  it("names who is being answered after Next, and how many wait", () => {
    const after = live(
      [answering("102"), queues.liveWatching.hands[2]],
      [closed("101", diego, "answered")],
    );
    expect(queueNews(queues.liveWatching, after)).toBe(
      "@ana-ferreira is being answered. 1 waiting.",
    );
  });

  it("FR-QUEUE-5: tells me my new place, and that I am next", () => {
    const oneAhead = live([answering("102"), hand("103", liwen), mine("104")]);
    expect(queueNews(queues.liveWaiting, oneAhead)).toBe(
      "@ana-ferreira is being answered. 1 ahead of you.",
    );
    const next = live([answering("103", liwen), mine("104")]);
    expect(queueNews(oneAhead, next)).toBe(
      "@liwen is being answered. You're next.",
    );
  });

  it("leaves my own turn to the your-turn banner, which says it at once", () => {
    const myTurn = live(
      [
        answering("104", diego, { isMine: true }),
        hand("102", ana),
        hand("103", liwen),
      ],
      [closed("101", ana, "answered")],
    );
    expect(queueNews(queues.liveNext, myTurn)).toBe("");
  });

  it("FR-QUEUE-8: says why I moved back after I missed my turn", () => {
    const back = live([
      hand("102", ana),
      hand("103", liwen),
      mine("101", { awayCount: 1 }),
    ]);
    expect(queueNews(queues.myTurn, back)).toBe(
      "You missed your turn, so you moved back. 2 ahead of you.",
    );
  });

  it("never removes someone silently", () => {
    const after = live(
      [answering("101"), hand("102", ana), hand("103", liwen)],
      [mine("104", { status: "removed", title: "" })],
    );
    expect(queueNews(queues.liveWaiting, after)).toBe(
      "A maintainer removed your hand. 2 waiting.",
    );
  });

  it("says what became of my hand, whatever ended it", () => {
    const cases: ReadonlyArray<readonly [HandStatus, string]> = [
      ["withdrawn", "You withdrew your hand."],
      ["skipped", "Your hand was skipped."],
      ["merged", "Your question joined the one being answered."],
    ];
    for (const [status, sentence] of cases) {
      const after = live(
        [answering("101"), hand("102", ana), hand("103", liwen)],
        [mine("104", { status })],
      );
      expect(queueNews(queues.liveWaiting, after)).toBe(
        `${sentence} 2 waiting.`,
      );
    }
  });

  it("tells me my question was answered", () => {
    const after = live(
      [hand("102", ana), hand("103", liwen)],
      [mine("101", { status: "answered" })],
    );
    expect(queueNews(queues.myTurn, after)).toBe("Your question was answered.");
  });

  it("FR-SESS-3: says that the session started", () => {
    expect(queueNews(queues.noSession, queues.liveEmpty)).toBe(
      "The session started: Thursday office hours.",
    );
  });

  it("FR-SESS-4, FR-SESS-7: says that the session ended, and what became of my question", () => {
    expect(queueNews(queues.liveWatching, queues.ended)).toBe(
      "The session ended.",
    );
    const carried: Queue = {
      ...queues.ended,
      done: [mine("104", { status: "carried" })],
    };
    expect(queueNews(queues.liveWaiting, carried)).toBe(
      "The session ended. Your question moves to the front of the next session.",
    );
    const notReached: Queue = {
      ...queues.ended,
      done: [mine("104", { status: "not_reached" })],
    };
    expect(queueNews(queues.liveWaiting, notReached)).toBe(
      "The session ended. Your question was not reached this time.",
    );
  });

  it("FR-SESS-6: says that hands closed, and that they opened again", () => {
    const lastCall: Queue = {
      ...queues.liveWatching,
      session: { ...liveSession, handsClosed: true },
    };
    expect(queueNews(queues.liveWatching, lastCall)).toBe("Hands are closed.");
    expect(queueNews(lastCall, queues.liveWatching)).toBe(
      "Hands are open again.",
    );
  });

  it("counts the waiting hands for someone without a hand", () => {
    const one = live([hand("102", ana)]);
    const two = live([hand("102", ana), hand("103", liwen)]);
    expect(queueNews(queues.liveEmpty, one)).toBe("1 waiting.");
    expect(queueNews(one, two)).toBe("2 waiting.");
    expect(queueNews(one, queues.liveEmpty)).toBe("Nobody is waiting.");
  });

  it("FR-QUEUE-5: tells me my place once I raised a hand", () => {
    const raised = live([...queues.liveWatching.hands, mine("104")]);
    expect(queueNews(queues.liveWatching, raised)).toBe("2 ahead of you.");
  });
});
