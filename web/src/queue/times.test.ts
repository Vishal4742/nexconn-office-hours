import { describe, expect, it } from "vitest";

import { toMs } from "./clock";
import { NOW, liveSession } from "./fixtures";
import { elapsedLabel, fromLocalInput, toLocalInput, whenLabel } from "./times";

const start = liveSession.startedAt;
const after = (minutes: number, seconds = 0) =>
  toMs(start) + minutes * 60_000 + seconds * 1_000;

describe("elapsedLabel", () => {
  it("says how many whole minutes the session has run", () => {
    expect(elapsedLabel(start, toMs(NOW))).toBe("20 min");
    expect(elapsedLabel(start, after(0, 59))).toBe("0 min");
    expect(elapsedLabel(start, after(1))).toBe("1 min");
    expect(elapsedLabel(start, after(59, 59))).toBe("59 min");
  });

  it("counts in hours from the first full hour", () => {
    expect(elapsedLabel(start, after(60))).toBe("1 h 00 min");
    expect(elapsedLabel(start, after(125))).toBe("2 h 05 min");
  });

  it("never counts backwards when a clock is a little off", () => {
    expect(elapsedLabel(start, after(0) - 4_000)).toBe("0 min");
  });

  it("says nothing for a start that is not a time", () => {
    expect(elapsedLabel("soon", toMs(NOW))).toBe("");
    expect(elapsedLabel(start, Number.NaN)).toBe("");
  });
});

describe("whenLabel", () => {
  const thursday = "2026-11-19T11:30:00.000Z";

  it("FR-SESS-5: gives the time in the viewer's own time zone", () => {
    const inIndia = whenLabel(thursday, "en-GB", "Asia/Kolkata");
    expect(inIndia).toContain("Thu");
    expect(inIndia).toContain("19 Nov");
    expect(inIndia).toContain("17:00");

    const inNewYork = whenLabel(thursday, "en-GB", "America/New_York");
    expect(inNewYork).toContain("Thu");
    expect(inNewYork).toContain("06:30");
  });

  it("FR-SESS-5: the day changes with the time zone too", () => {
    const lateEvening = "2026-11-19T20:00:00.000Z";
    expect(whenLabel(lateEvening, "en-GB", "Asia/Kolkata")).toContain("Fri");
    expect(whenLabel(lateEvening, "en-GB", "Asia/Kolkata")).toContain("01:30");
  });

  it("says nothing for what is not a time", () => {
    expect(whenLabel("soon", "en-GB", "Asia/Kolkata")).toBe("");
  });
});

describe("the value of a datetime-local field", () => {
  it("is the viewer's wall clock to the minute, and reads back as the same moment", () => {
    for (const time of [
      "2026-11-19T11:30:00.000Z",
      "2026-01-01T00:00:00.000Z",
      "2026-12-31T23:59:00.000Z",
      "2027-03-07T08:05:00.000Z",
    ]) {
      const value = toLocalInput(time);
      expect(value).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
      expect(fromLocalInput(value)?.toISOString()).toBe(time);
    }
  });

  it("drops the seconds: a session is planned to the minute", () => {
    const value = toLocalInput("2026-11-19T11:30:42.123456Z");
    expect(fromLocalInput(value)?.toISOString()).toBe(
      "2026-11-19T11:30:00.000Z",
    );
  });

  it("is empty for what is not a time, and an empty or broken field is no time", () => {
    expect(toLocalInput("soon")).toBe("");
    expect(fromLocalInput("")).toBeNull();
    expect(fromLocalInput("next thursday")).toBeNull();
  });
});
