import { describe, expect, it } from "vitest";

import { isFirefox } from "./browser";

describe("isFirefox", () => {
  it("NFR-COMP-1: finds Firefox, and only Firefox", () => {
    expect(
      isFirefox(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:132.0) Gecko/20100101 Firefox/132.0",
      ),
    ).toBe(true);
    expect(
      isFirefox(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
      ),
    ).toBe(false);
    expect(
      isFirefox(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/132.0 Mobile/15E148 Safari/605.1.15",
      ),
    ).toBe(false);
  });
});
