import { describe, expect, it } from "vitest";

import { mayReload } from "./staleCode";

describe("reloading a page whose code is gone from the server", () => {
  it("reloads the first time, when online", () => {
    expect(mayReload(null, true)).toBe(true);
  });

  it("never twice in one tab: a file that keeps failing must not loop the page (state-machine audit, bug 4)", () => {
    expect(mayReload("1800000000000", true)).toBe(false);
    expect(mayReload("", true)).toBe(false);
    expect(mayReload("not a time", true)).toBe(false);
  });

  it("never while offline: the reload would only show the browser's offline page", () => {
    expect(mayReload(null, false)).toBe(false);
  });
});
