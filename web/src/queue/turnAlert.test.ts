import { describe, expect, it } from "vitest";

import { ana, diego, hand, liveSession, mia, queues } from "./fixtures";
import { turnAlert } from "./turnAlert";
import type { Hand, Queue } from "./types";

/** A live queue: the hand being answered, if any, then the waiting line in the server's order. */
const live = (answering: Hand | null, waiting: readonly Hand[]): Queue => ({
  ...queues.liveEmpty,
  hands: [...(answering ? [answering] : []), ...waiting],
});
const mine = (over: Partial<Hand> = {}) =>
  hand("200", diego, { isMine: true, title: "Can I pin a version?", ...over });
const someone = (id: string, over: Partial<Hand> = {}) => hand(id, ana, over);
const answering = (h: Hand) => ({
  ...h,
  status: "answering" as const,
  takenBy: mia,
});

describe("turnAlert", () => {
  it("FR-NOTIF-2: says it's my turn when my hand starts being answered", () => {
    const before = live(answering(someone("150")), [mine()]);
    const after = live(answering(mine()), []);
    expect(turnAlert(before, after)).toEqual({
      kind: "turn",
      hand: after.hands[0],
    });
  });

  it("says I'm next when my hand becomes first in line", () => {
    const before = live(answering(someone("150")), [someone("201"), mine()]);
    const after = live(answering(someone("201")), [mine()]);
    expect(turnAlert(before, after)).toEqual({
      kind: "next",
      hand: after.hands[1],
    });
  });

  it("says each thing once: a later queue in the same state says nothing", () => {
    const next = live(answering(someone("150")), [mine()]);
    const turn = live(answering(mine()), [someone("202")]);
    expect(
      turnAlert(next, { ...next, serverTime: "2026-11-14T11:51:00Z" }),
    ).toBeNull();
    expect(
      turnAlert(turn, { ...turn, serverTime: "2026-11-14T11:52:00Z" }),
    ).toBeNull();
  });

  it("says nothing on the first queue: the banner and the tab title already show it", () => {
    expect(turnAlert(null, live(answering(mine()), []))).toBeNull();
    expect(turnAlert(null, live(null, [mine()]))).toBeNull();
  });

  it("says nothing while I wait further back, or about someone else's turn", () => {
    const before = live(answering(someone("150")), [
      someone("201"),
      someone("202"),
      mine(),
    ]);
    const after = live(answering(someone("201")), [someone("202"), mine()]);
    expect(turnAlert(before, after)).toBeNull();
    expect(
      turnAlert(
        live(null, [someone("201")]),
        live(answering(someone("201")), []),
      ),
    ).toBeNull();
  });

  it("says nothing when I have no hand open", () => {
    const before = live(answering(someone("150")), [someone("201")]);
    expect(turnAlert(before, live(answering(someone("201")), []))).toBeNull();
  });

  it("says I'm next when a new session starts with my carried hand first", () => {
    const ended: Queue = {
      ...queues.noSession,
      session: { ...liveSession, live: false, endedAt: "2026-11-14T12:15:00Z" },
      done: [mine({ status: "carried" })],
    };
    const started = live(null, [mine()]);
    expect(turnAlert(ended, started)?.kind).toBe("next");
  });
});
