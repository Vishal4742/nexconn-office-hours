import { describe, expect, it } from "vitest";

import { clockOffset, makeClock, remember, toMs } from "./clock";

describe("toMs", () => {
  const second = Date.UTC(2026, 10, 14, 11, 50, 0);

  it("reads the server's times, however many digits follow the second", () => {
    expect(toMs("2026-11-14T11:50:00Z")).toBe(second);
    expect(toMs("2026-11-14T11:50:00.1Z")).toBe(second + 100);
    expect(toMs("2026-11-14T11:50:00.123Z")).toBe(second + 123);
    expect(toMs("2026-11-14T11:50:00.123456Z")).toBe(second + 123);
    expect(toMs("2026-11-14T11:50:00.123456789Z")).toBe(second + 123);
    expect(toMs("2026-11-14T17:20:00.123456+05:30")).toBe(second + 123);
  });

  it("is not a number for what is not a time", () => {
    expect(toMs("")).toBeNaN();
    expect(toMs("soon")).toBeNaN();
  });
});

describe("clockOffset", () => {
  it("plan decision 12: sets the server's time against the middle of the fetch", () => {
    // The page's clock is 5 s behind. It asked at 1,000 and was answered at 1,400; in the
    // middle, at 1,200, the server's clock said 6,200.
    expect(clockOffset(new Date(6_200).toISOString(), 1_000, 1_400)).toBe(
      5_000,
    );
  });

  it("is negative for a page whose clock is ahead", () => {
    expect(clockOffset(new Date(1_200).toISOString(), 61_000, 61_400)).toBe(
      -60_000,
    );
  });

  it("is nothing when the answer holds no time that can be read", () => {
    expect(clockOffset("soon", 1_000, 1_400)).toBeNull();
  });
});

describe("makeClock", () => {
  it("counts on the page's clock until a fetch was answered", () => {
    const clock = makeClock(() => 1_000);
    expect(clock.now()).toBe(1_000);
    expect(clock.serverNow()).toBe(1_000);
  });

  it("plan decision 12: the server's time is the page's clock plus the difference it kept", () => {
    let page = 1_000;
    const clock = makeClock(() => page);
    clock.sync(new Date(6_200).toISOString(), 1_000, 1_400);
    page = 2_000;
    expect(clock.now()).toBe(2_000);
    expect(clock.serverNow()).toBe(7_000);
  });

  it("keeps the difference it has when an answer holds no readable time", () => {
    const clock = makeClock(() => 1_000);
    clock.sync(new Date(6_200).toISOString(), 1_000, 1_400);
    clock.sync("soon", 1_000, 1_400);
    expect(clock.serverNow()).toBe(6_000);
  });
});

describe("remember", () => {
  it("NFR-PERF-1: keeps the last 50 latencies, the newest last", () => {
    let kept: readonly number[] = [];
    for (let ms = 1; ms <= 60; ms++) kept = remember(kept, ms);
    expect(kept).toHaveLength(50);
    expect(kept[0]).toBe(11);
    expect(kept[49]).toBe(60);
  });

  it("does not change the list it was given", () => {
    const before = [1, 2] as const;
    expect(remember(before, 3)).toEqual([1, 2, 3]);
    expect(before).toEqual([1, 2]);
  });
});
