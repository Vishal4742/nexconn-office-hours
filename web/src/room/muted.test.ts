import { describe, expect, it } from "vitest";

import { mutedLine } from "./muted";

const at = (iso: string) => new Date(iso).getTime();

describe("mutedLine (FR-MOD-1, phase-5 finding S3)", () => {
  it("tells a muted member until when, in their own time, and that they can still read", () => {
    expect(
      mutedLine(
        "2026-11-17T09:52:00.000Z",
        at("2026-11-17T09:42:00Z"),
        "en-GB",
        "Asia/Kolkata",
      ),
    ).toBe(
      "A maintainer muted you in this room until 15:22. You can still read it.",
    );
  });

  it("says nothing without a mute, once it has ended, or for a time that isn't one", () => {
    const now = at("2026-11-17T09:42:00Z");
    expect(mutedLine(null, now, "en-GB", "UTC")).toBeNull();
    expect(
      mutedLine("2026-11-17T09:42:00.000Z", now, "en-GB", "UTC"),
    ).toBeNull();
    expect(
      mutedLine("2026-11-17T09:30:00.000Z", now, "en-GB", "UTC"),
    ).toBeNull();
    expect(mutedLine("soon", now, "en-GB", "UTC")).toBeNull();
  });
});
