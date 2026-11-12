import { describe, expect, it } from "vitest";

import { BRUSH_EDGE, BRUSH_WIDTH, brushMaskUrl, brushRows } from "./brush";

describe("the hero's painted swath", () => {
  it("is 75 thin bristle rows, one under the other", () => {
    const rows = brushRows();
    expect(rows).toHaveLength(75);
    rows.forEach((row, index) => expect(row.y).toBe(20 + index * 8));
  });

  it("keeps every ragged end inside the photo, since a mask cannot paint outside it", () => {
    for (const row of brushRows()) {
      expect(row.x).toBeGreaterThanOrEqual(BRUSH_EDGE);
      expect(row.x + row.width).toBeLessThanOrEqual(
        BRUSH_WIDTH - BRUSH_EDGE + 1e-9,
      );
      expect(row.width).toBeGreaterThan(BRUSH_WIDTH / 3);
    }
  });

  it("has ragged ends: the rows do not all start or stop at the same place", () => {
    const rows = brushRows();
    expect(new Set(rows.map((row) => row.x)).size).toBeGreaterThan(60);
    expect(new Set(rows.map((row) => row.x + row.width)).size).toBeGreaterThan(
      60,
    );
  });

  it("paints the same swath on every load", () => {
    expect(brushRows()).toEqual(brushRows());
    expect(brushMaskUrl()).toBe(brushMaskUrl());
  });

  it('is a CSS image whose own characters cannot break out of url("…")', () => {
    const url = brushMaskUrl();
    const prefix = 'url("data:image/svg+xml,';
    expect(url.startsWith(prefix)).toBe(true);
    expect(url.endsWith('")')).toBe(true);
    const data = url.slice(prefix.length, -2);
    // A quote or backslash would end the string early; "#" would cut the address at a fragment.
    expect(data).not.toMatch(/["\\#\s<>]/);
    const svg = decodeURIComponent(data);
    expect(svg).toContain('xmlns="http://www.w3.org/2000/svg"');
    expect(svg.split("<rect ").length - 1).toBe(75);
    expect(svg).toContain('filter="url(#rough)"');
  });
});
