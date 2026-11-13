import { describe, expect, it } from "vitest";

import { NOW, liveSession, queues } from "./fixtures";
import { questionTimer, tabTitle, waitLabel } from "./polish";

const BASE = "PaperPlane/Stash · Office Hours";

describe("tabTitle", () => {
  it("FR-QUEUE-11: carries my place in the line", () => {
    expect(tabTitle(BASE, queues.liveWaiting)).toBe(`(2 ahead) ${BASE}`);
    expect(tabTitle(BASE, queues.liveNext)).toBe(`(You're next) ${BASE}`);
    expect(tabTitle(BASE, queues.myTurn)).toBe(`(Your turn) ${BASE}`);
  });

  it("is the plain title without a hand of mine or a live session", () => {
    expect(tabTitle(BASE, queues.liveWatching)).toBe(BASE);
    expect(tabTitle(BASE, queues.noSession)).toBe(BASE);
    expect(tabTitle(BASE, queues.ended)).toBe(BASE);
  });
});

describe("waitLabel", () => {
  it("FR-QUEUE-10: is said only once two answers give a median", () => {
    expect(waitLabel(2, 0, true)).toBe("");
  });

  it("counts the hands ahead, and the one being answered, at the median", () => {
    expect(waitLabel(2, 300, true)).toBe("about 15 min"); // 2 ahead + the current: 3 × 5 min
    expect(waitLabel(2, 300, false)).toBe("about 10 min");
    expect(waitLabel(0, 300, true)).toBe("about 5 min");
  });

  it("never says less than a minute, and says nothing when it is my turn already", () => {
    expect(waitLabel(0, 20, false)).toBe("");
    expect(waitLabel(1, 20, false)).toBe("about 1 min");
  });
});

describe("questionTimer", () => {
  const took = (minutes: number, seconds = 0) =>
    Date.parse(liveSession.startedAt) + (minutes * 60 + seconds) * 1000;

  it("FR-QUEUE-12: counts minutes and seconds since the hand was taken", () => {
    expect(questionTimer(liveSession.startedAt, took(4, 7))).toEqual({
      label: "4:07",
      isLong: false,
    });
  });

  it("turns long after 8 minutes", () => {
    expect(questionTimer(liveSession.startedAt, took(7, 59)).isLong).toBe(
      false,
    );
    expect(questionTimer(liveSession.startedAt, took(8, 0))).toEqual({
      label: "8:00",
      isLong: true,
    });
    expect(questionTimer(liveSession.startedAt, took(64, 5)).label).toBe(
      "64:05",
    );
  });

  it("shows nothing for a time that can't be read, and never runs backwards", () => {
    expect(questionTimer(null, Date.parse(NOW)).label).toBe("");
    expect(questionTimer(liveSession.startedAt, took(-1)).label).toBe("0:00");
  });
});
