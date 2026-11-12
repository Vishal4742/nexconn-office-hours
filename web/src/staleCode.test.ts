import { describe, expect, it } from "vitest";

import { mayReload } from "./staleCode";

describe("reloading a page whose code is gone from the server", () => {
  const now = 1_800_000_000_000;

  it("reloads the first time", () => {
    expect(mayReload(null, now)).toBe(true);
    expect(mayReload("", now)).toBe(true);
    expect(mayReload("not a time", now)).toBe(true);
  });

  it("does not reload again within ten seconds: a file that keeps failing must not loop the page", () => {
    expect(mayReload(String(now - 1_000), now)).toBe(false);
    expect(mayReload(String(now - 9_999), now)).toBe(false);
    expect(mayReload(String(now - 10_001), now)).toBe(true);
  });
});
