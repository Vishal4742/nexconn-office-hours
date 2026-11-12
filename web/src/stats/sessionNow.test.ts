import { describe, expect, it } from "vitest";

import { liveSession, queues } from "../queue/fixtures";
import type { Queue } from "../queue/types";
import { sessionNow, sessionTime } from "./sessionNow";

// Thursday 19 November 2026, 11:30 in UTC: 17:00 in India, 03:30 in California.
const THURSDAY = "2026-11-19T11:30:00.000Z";

describe("the next session's time", () => {
  // The words between the parts are the browser's own and differ a little between versions, so
  // these check the parts and their order, not every comma.
  it("FR-SESS-5: is told in the viewer's own time zone", () => {
    expect(sessionTime(THURSDAY, "en-GB", "UTC")).toMatch(
      /^Thu,? 19 Nov,? 11:30$/,
    );
    expect(sessionTime(THURSDAY, "en-GB", "Asia/Kolkata")).toMatch(
      /^Thu,? 19 Nov,? 17:00$/,
    );
    expect(sessionTime(THURSDAY, "en-GB", "America/Los_Angeles")).toMatch(
      /^Thu,? 19 Nov,? 03:30$/,
    );
  });

  it("FR-SESS-5: the day is the viewer's day too", () => {
    const lateEvening = "2026-11-19T22:30:00.000Z";
    expect(sessionTime(lateEvening, "en-GB", "Asia/Tokyo")).toMatch(
      /^Fri,? 20 Nov,? 07:30$/,
    );
    expect(sessionTime(lateEvening, "en-GB", "America/New_York")).toMatch(
      /^Thu,? 19 Nov,? 17:30$/,
    );
  });

  it("FR-SESS-5: follows the viewer's way of writing a date", () => {
    expect(sessionTime(THURSDAY, "en-US", "America/Los_Angeles")).toMatch(
      /^Thu,? Nov 19,? 0?3:30\sAM$/,
    );
  });

  it("is nothing for a time that cannot be read", () => {
    expect(sessionTime("soon", "en-GB", "UTC")).toBe("");
    expect(sessionTime("", "en-GB", "UTC")).toBe("");
  });
});

describe("what the join page says about the session", () => {
  const inIndia = (queue: Queue) => sessionNow(queue, "en-GB", "Asia/Kolkata");

  it("FR-PROJ-3: a live session, with its title", () => {
    expect(inIndia(queues.liveEmpty)).toEqual({
      kind: "live",
      title: "Thursday office hours",
    });
  });

  it("FR-PROJ-3: someone outside the room gets the session and no hands, and that is enough", () => {
    const outside: Queue = { ...queues.liveWatching, hands: [], done: [] };
    expect(inIndia(outside).kind).toBe("live");
  });

  it("FR-PROJ-3, FR-SESS-5: the next session's time when none is live", () => {
    const now = inIndia(queues.nextPlanned);
    expect(now).toMatchObject({ kind: "next", at: THURSDAY });
    expect(now.kind === "next" && now.when).toMatch(/^Thu,? 19 Nov,? 17:00$/);
    // The last session ended and the next one is planned.
    expect(inIndia(queues.ended).kind).toBe("next");
  });

  it("a live session matters more than the next one", () => {
    const both: Queue = { ...queues.nextPlanned, session: liveSession };
    expect(inIndia(both).kind).toBe("live");
  });

  it("says nothing is planned when the project has no session and no date", () => {
    expect(inIndia(queues.noSession)).toEqual({ kind: "none" });
  });

  it("FR-SESS-5: a planned time that has passed is no longer the next session", () => {
    const passed: Queue = {
      ...queues.noSession,
      nextSessionAt: "2026-11-14T11:30:00.000Z", // twenty minutes before the server's clock
    };
    const rightNow: Queue = {
      ...queues.noSession,
      nextSessionAt: queues.noSession.serverTime,
    };
    expect(inIndia(passed)).toEqual({ kind: "none" });
    expect(inIndia(rightNow)).toEqual({ kind: "none" });
  });

  it("counts from the server's clock, not the page's", () => {
    // Planned for 2020: long ago for this machine, still ahead for the server that answered.
    const ahead: Queue = {
      ...queues.noSession,
      nextSessionAt: "2020-01-02T10:00:00.000Z",
      serverTime: "2020-01-01T10:00:00.000Z",
    };
    expect(inIndia(ahead).kind).toBe("next");
  });

  it("a time that cannot be read is no next session", () => {
    expect(inIndia({ ...queues.noSession, nextSessionAt: "soon" })).toEqual({
      kind: "none",
    });
    expect(
      inIndia({ ...queues.nextPlanned, serverTime: "not a time" }),
    ).toEqual({ kind: "none" });
  });
});
