import { describe, expect, it } from "vitest";

import {
  DOT_FONT,
  DOT_FULL,
  DOT_GRID,
  WAVE_SPREAD,
  clockDots,
  coverRect,
  dotRadius,
  pointIn,
  waveDots,
} from "./dots";

const DIGITS = [..."0123456789"];

/** How many dots a glyph lights. */
function litIn(rows: readonly string[] | undefined): number {
  return (rows ?? []).join("").split("1").length - 1;
}

describe("the clock's 5×7 dot font", () => {
  it("draws every digit on five columns and seven rows, and the colon as one column", () => {
    for (const digit of DIGITS) {
      const rows = DOT_FONT[digit];
      expect(rows, digit).toHaveLength(7);
      for (const row of rows ?? []) expect(row, digit).toMatch(/^[01]{5}$/);
    }
    expect(DOT_FONT[":"]).toEqual(["0", "0", "1", "0", "1", "0", "0"]);
  });

  it("draws the digits of the story the way they read", () => {
    expect(DOT_FONT["1"]).toEqual([
      "00100",
      "01100",
      "00100",
      "00100",
      "00100",
      "00100",
      "01110",
    ]);
    expect(DOT_FONT["7"]).toEqual([
      "11111",
      "00001",
      "00010",
      "00100",
      "01000",
      "01000",
      "01000",
    ]);
    expect(DOT_FONT["4"]).toEqual([
      "00010",
      "00110",
      "01010",
      "10010",
      "11111",
      "00010",
      "00010",
    ]);
  });

  it("gives every digit a pattern of its own", () => {
    const patterns = new Set(DIGITS.map((digit) => DOT_FONT[digit]?.join("/")));
    expect(patterns.size).toBe(10);
  });
});

describe("the dot-matrix clock", () => {
  it("lights the dots of each character of HH:MM, each in its own columns", () => {
    const dots = clockDots("17:45");
    // Four digits of five columns, a colon of one: 21 columns of seven dots.
    expect(dots).toHaveLength(21 * 7);
    expect(dots.filter((dot) => dot.isOn)).toHaveLength(
      litIn(DOT_FONT["1"]) +
        litIn(DOT_FONT["7"]) +
        litIn(DOT_FONT[":"]) +
        litIn(DOT_FONT["4"]) +
        litIn(DOT_FONT["5"]),
    );
    // Columns 0-4, 6-10, the colon in 12, then 14-18 and 20-24; 8 px apart, centred.
    const colon = dots.filter((dot) => dot.cx === 12 * 8 + 4);
    expect(colon.filter((dot) => dot.isOn).map((dot) => dot.cy)).toEqual([
      20, 36,
    ]);
    // The "1" lights its middle column on the top row, and not its corners.
    const topRow = dots.filter((dot) => dot.cy === 4 && dot.cx < 40);
    expect(topRow.map((dot) => dot.isOn)).toEqual([
      false,
      false,
      true,
      false,
      false,
    ]);
  });

  it("keeps every dot in its place for every time, so a change flips dots instead of redrawing the face", () => {
    expect(clockDots("17:00").map((dot) => dot.key)).toEqual(
      clockDots("17:45").map((dot) => dot.key),
    );
    expect(new Set(clockDots("17:12").map((dot) => dot.key)).size).toBe(21 * 7);
  });

  it("fits the 200 by 56 face and flips every dot within a fifth of a second", () => {
    for (const dot of clockDots("17:31")) {
      expect(dot.cx).toBeGreaterThan(0);
      expect(dot.cx).toBeLessThan(200);
      expect(dot.cy).toBeGreaterThan(0);
      expect(dot.cy).toBeLessThan(56);
      expect(dot.delay).toBeGreaterThanOrEqual(0);
      expect(dot.delay).toBeLessThan(220);
    }
  });
});

describe("the photo's crop in the story frame", () => {
  const photo = { width: 1536, height: 864 };

  it("fills a tall frame by its height and slides the photo sideways by object-position", () => {
    const rect = coverRect({ width: 644, height: 720 }, photo, {
      x: 0.68,
      y: 0.6,
    });
    expect(rect.width).toBeCloseTo(1280);
    expect(rect.height).toBeCloseTo(720);
    expect(rect.x).toBeCloseTo((644 - 1280) * 0.68);
    expect(rect.y).toBeCloseTo(0);
  });

  it("fills a wide frame by its width and slides the photo up", () => {
    const rect = coverRect({ width: 1600, height: 600 }, photo, {
      x: 0.68,
      y: 0.6,
    });
    expect(rect.width).toBeCloseTo(1600);
    expect(rect.height).toBeCloseTo(900);
    expect(rect.x).toBeCloseTo(0);
    expect(rect.y).toBeCloseTo(-180);
  });

  it("finds a point of the photo where it lands in the frame", () => {
    const rect = { x: -100, y: -50, width: 3072, height: 1728 };
    expect(pointIn(rect, photo, { x: 768, y: 432 })).toEqual({
      x: 1436,
      y: 814,
    });
  });
});

describe("the dot wave", () => {
  const box = { width: 644, height: 720 };
  const lamp = { x: 440, y: 408 }; // a dot of the grid sits right on it
  const dots = waveDots(box, lamp);
  const distance = (dot: { readonly x: number; readonly y: number }) =>
    Math.hypot(dot.x - lamp.x, dot.y - lamp.y);

  it("switches the dots on in order of their distance from the lamp", () => {
    expect(dots[0]).toEqual({ x: 440, y: 408, start: 0 });
    for (let index = 1; index < dots.length; index += 1) {
      const dot = dots[index];
      const before = dots[index - 1];
      if (!dot || !before) throw new Error("a dot is missing");
      expect(dot.start).toBeGreaterThanOrEqual(before.start);
      expect(distance(dot)).toBeGreaterThanOrEqual(distance(before) - 1e-9);
    }
    expect(dots.at(-1)?.start).toBeCloseTo(WAVE_SPREAD);
  });

  it("covers every corner of the frame once the dots are full, so the picture is whole at the end", () => {
    const corners = [
      { x: 0, y: 0 },
      { x: box.width, y: 0 },
      { x: 0, y: box.height },
      { x: box.width, y: box.height },
    ];
    for (const corner of corners) {
      const nearest = Math.min(
        ...dots.map((dot) => Math.hypot(dot.x - corner.x, dot.y - corner.y)),
      );
      expect(nearest).toBeLessThanOrEqual(DOT_FULL);
    }
    // Inside, the farthest a pixel can be from a dot is half a cell's diagonal.
    expect(DOT_FULL).toBeGreaterThanOrEqual((DOT_GRID / 2) * Math.SQRT2);
  });

  it("grows each dot from nothing to full, a later dot never bigger than an earlier one", () => {
    expect(dotRadius(0, 0)).toBe(0);
    expect(dotRadius(WAVE_SPREAD, WAVE_SPREAD)).toBe(0);
    expect(dotRadius(1, WAVE_SPREAD)).toBe(DOT_FULL);
    for (const t of [0.1, 0.3, 0.5, 0.7, 0.9]) {
      expect(dotRadius(t, 0.2)).toBeLessThanOrEqual(dotRadius(t, 0.1));
      expect(dotRadius(t, 0.1)).toBeLessThanOrEqual(dotRadius(t + 0.05, 0.1));
    }
    for (const dot of dots) expect(dotRadius(1, dot.start)).toBe(DOT_FULL);
  });
});
