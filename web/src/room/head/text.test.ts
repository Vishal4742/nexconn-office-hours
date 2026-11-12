import { describe, expect, it } from "vitest";

import { NOW, liveSession, queues } from "../../queue/fixtures";
import type { QueueSession } from "../../queue/types";
import { LIVE_LABEL, headline, liveState, metaLine, minutesLeft } from "./text";

const at = (time: string) => Date.parse(`2026-11-14T${time}Z`);
const now = Date.parse(NOW);

describe("liveState", () => {
  it("FR-SESS-6: says Live, Last call or Not live for each state of the queue", () => {
    const said = Object.fromEntries(
      Object.entries(queues).map(([name, queue]) => [
        name,
        LIVE_LABEL[liveState(queue.session)],
      ]),
    );
    expect(said).toEqual({
      noSession: "Not live",
      nextPlanned: "Not live",
      liveEmpty: "Live",
      liveWatching: "Live",
      liveWaiting: "Live",
      liveNext: "Live",
      myTurn: "Live",
      lastCall: "Last call",
      ended: "Not live",
    });
  });
});

describe("headline", () => {
  it("is the session with its state as the last word, for each state of the queue", () => {
    expect(headline(queues.noSession.session)).toEqual({
      name: "No session",
      state: "right now.",
    });
    expect(headline(queues.liveWatching.session)).toEqual({
      name: "Thursday office hours,",
      state: "live.",
    });
    expect(headline(queues.lastCall.session)).toEqual({
      name: "Thursday office hours,",
      state: "last call.",
    });
    expect(headline(queues.ended.session)).toEqual({
      name: "Thursday office hours,",
      state: "ended.",
    });
  });

  it("does not put a comma after the title's own last mark", () => {
    const named = (title: string) => headline({ ...liveSession, title }).name;
    expect(named("Ask me anything!")).toBe("Ask me anything,");
    expect(named("Release Q&A...  ")).toBe("Release Q&A,");
    expect(named("?????")).toBe("?????,");
  });
});

describe("minutesLeft", () => {
  it("counts down from the planned length and below zero once it is over", () => {
    expect(minutesLeft(liveSession, now)).toBe(25);
    expect(minutesLeft(liveSession, at("12:14:59.999"))).toBe(1);
    expect(minutesLeft(liveSession, at("12:15:00"))).toBe(0);
    expect(minutesLeft(liveSession, at("12:20:30"))).toBe(-5);
  });

  it("is null without a planned length, and never more than the length when the clocks differ", () => {
    expect(minutesLeft({ ...liveSession, plannedMinutes: 0 }, now)).toBeNull();
    expect(minutesLeft(liveSession, at("11:29:00"))).toBe(45);
    expect(minutesLeft({ ...liveSession, startedAt: "soon" }, now)).toBeNull();
  });
});

describe("metaLine", () => {
  it("is the session's number, length, host and minutes left, for each state of the queue", () => {
    const lines = Object.fromEntries(
      Object.entries(queues).map(([name, queue]) => [
        name,
        metaLine(queue.session, now),
      ]),
    );
    expect(lines).toEqual({
      noSession: "",
      nextPlanned: "",
      liveEmpty: "Nº 3 · 45 min with @mia-k · 25 min left",
      liveWatching: "Nº 3 · 45 min with @mia-k · 25 min left",
      liveWaiting: "Nº 3 · 45 min with @mia-k · 25 min left",
      liveNext: "Nº 3 · 45 min with @mia-k · 25 min left",
      myTurn: "Nº 3 · 45 min with @mia-k · 25 min left",
      lastCall: "Nº 3 · 45 min with @mia-k · 25 min left",
      ended: "Nº 3 · with @mia-k · ended after 45 min",
    });
  });

  it("says when the time is up and how many minutes it is over", () => {
    expect(metaLine(liveSession, at("12:15:00"))).toBe(
      "Nº 3 · 45 min with @mia-k · time is up",
    );
    expect(metaLine(liveSession, at("12:20:00"))).toBe(
      "Nº 3 · 45 min with @mia-k · 5 min over",
    );
  });

  it("counts minutes in when no length was planned, and leaves out what it does not know", () => {
    const open: QueueSession = { ...liveSession, plannedMinutes: 0 };
    expect(metaLine(open, now)).toBe("Nº 3 · with @mia-k · 20 min in");
    expect(metaLine({ ...open, host: null }, now)).toBe("Nº 3 · 20 min in");
    expect(metaLine({ ...liveSession, host: null }, now)).toBe(
      "Nº 3 · 45 min · 25 min left",
    );
    expect(metaLine(liveSession, null)).toBe("Nº 3 · 45 min with @mia-k");
    expect(metaLine({ ...queues.ended.session, endedAt: null }, now)).toBe(
      "Nº 3 · with @mia-k · ended",
    );
  });
});
