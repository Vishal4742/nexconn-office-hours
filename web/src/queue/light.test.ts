import { describe, expect, it } from "vitest";

import { liveSession } from "./fixtures";
import type { QueueSession } from "./types";
import { sessionLight } from "./light";

const START = Date.parse(liveSession.startedAt);
const at = (minutes: number) => START + minutes * 60_000;

describe("sessionLight", () => {
  it("docs/UI.md S3: no session is the afternoon", () => {
    expect(sessionLight(null, at(0))).toBe("day");
  });

  it("follows the share of the planned length: afternoon, golden hour, dusk, night", () => {
    // 45 minutes planned: 35 % is 15.75 min, 70 % is 31.5 min, 95 % is 42.75 min.
    const cases: readonly (readonly [number, string])[] = [
      [0, "day"],
      [15.7, "day"],
      [15.8, "golden"],
      [31.4, "golden"],
      [31.6, "dusk"],
      [42.7, "dusk"],
      [42.8, "night"],
      [90, "night"],
    ];
    for (const [minutes, light] of cases) {
      expect(sessionLight(liveSession, at(minutes)), `${minutes} min`).toBe(
        light,
      );
    }
  });

  it("counts 45 minutes when no length was planned", () => {
    const open: QueueSession = { ...liveSession, plannedMinutes: 0 };
    expect(sessionLight(open, at(15.7))).toBe("day");
    expect(sessionLight(open, at(42.8))).toBe("night");
  });

  it("uses the planned length when there is one", () => {
    const long: QueueSession = { ...liveSession, plannedMinutes: 120 };
    expect(sessionLight(long, at(40))).toBe("day");
    expect(sessionLight(long, at(60))).toBe("golden");
  });

  it("last call and an ended session are night", () => {
    expect(sessionLight({ ...liveSession, handsClosed: true }, at(1))).toBe(
      "night",
    );
    expect(sessionLight({ ...liveSession, live: false }, at(1))).toBe("night");
  });

  it("a clock that is behind the start, or a time that can't be read, is the afternoon", () => {
    expect(sessionLight(liveSession, at(-5))).toBe("day");
    expect(
      sessionLight({ ...liveSession, startedAt: "not a time" }, at(10)),
    ).toBe("day");
  });
});
