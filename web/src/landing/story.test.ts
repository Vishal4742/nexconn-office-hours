import { describe, expect, it } from "vitest";

import { coverRect, pointIn } from "./dots";
import {
  FRAME_CROP,
  PHOTOS,
  PHOTO_SIZE,
  STORY,
  nextStep,
  withEntries,
} from "./story";

describe("the story of one session (docs/UI.md S1)", () => {
  it("follows one session by the clock, from the afternoon to the night", () => {
    expect(STORY.map((step) => step.time)).toEqual([
      "17:00",
      "17:04",
      "17:12",
      "17:31",
      "17:45",
    ]);
    expect(STORY.map((step) => step.heading)).toEqual([
      "Doors open",
      "A hand goes up",
      "It's your turn",
      "Answered once",
      "Last call",
    ]);
    expect(STORY.map((step) => step.photo)).toEqual([
      "afternoon",
      "afternoon",
      "golden",
      "dusk",
      "night",
    ]);
  });

  it("shows a different piece of the product at every step", () => {
    expect(new Set(STORY.map((step) => step.moment)).size).toBe(STORY.length);
  });

  it("keeps every lamp inside the frame at every size the page draws it, so the wave starts on the picture", () => {
    // The frame is the wide column of the story: 469 px wide in a 900 px window, 644 px from
    // 1200 px; it is at most 720 px tall, and 472 px in a 600 px tall window.
    for (const box of [
      { width: 469, height: 472 },
      { width: 469, height: 720 },
      { width: 644, height: 472 },
      { width: 644, height: 720 },
    ]) {
      const picture = coverRect(box, PHOTO_SIZE, FRAME_CROP);
      for (const [name, photo] of Object.entries(PHOTOS)) {
        const lamp = pointIn(picture, PHOTO_SIZE, photo.lamp);
        const where = `${name} in ${box.width}×${box.height}`;
        expect(lamp.x, where).toBeGreaterThan(0);
        expect(lamp.x, where).toBeLessThan(box.width);
        expect(lamp.y, where).toBeGreaterThan(0);
        expect(lamp.y, where).toBeLessThan(box.height);
      }
    }
  });
});

describe("which step is current", () => {
  it("stays where it is while no step crosses the middle of the window", () => {
    expect(nextStep(0, new Set())).toBe(0);
    expect(nextStep(3, new Set())).toBe(3);
  });

  it("keeps the current step while it still crosses the middle, so a reader resting on a boundary sees no flicker", () => {
    expect(nextStep(1, new Set([1, 2]))).toBe(1);
    expect(nextStep(2, new Set([1, 2]))).toBe(2);
    expect(nextStep(4, new Set([4]))).toBe(4);
  });

  it("moves to the step crossing the middle once the current one has left it, scrolling down or up", () => {
    expect(nextStep(1, new Set([2]))).toBe(2);
    expect(nextStep(2, new Set([1]))).toBe(1);
  });

  it("after a jump, takes the crossing step nearest to where the reader was", () => {
    expect(nextStep(0, new Set([3, 4]))).toBe(3);
    expect(nextStep(4, new Set([0, 1]))).toBe(1);
    expect(nextStep(2, new Set([4, 0]))).toBe(0);
  });
});

describe("the steps crossing the middle of the window", () => {
  it("follow the observer's entries, and the set it had is left as it was", () => {
    const before: ReadonlySet<number> = new Set([1]);
    const after = withEntries(before, [
      { index: 2, isCrossing: true },
      { index: 1, isCrossing: false },
    ]);
    expect([...after]).toEqual([2]);
    expect([...before]).toEqual([1]);
  });

  it("ignore a step that leaves without having been seen", () => {
    expect([
      ...withEntries(new Set(), [{ index: 3, isCrossing: false }]),
    ]).toEqual([]);
  });
});
