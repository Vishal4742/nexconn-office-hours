import { describe, expect, it } from "vitest";

import { signInHref, signInMessage } from "./auth";

describe("sign-in links and messages", () => {
  it("FR-AUTH-1: the sign-in link carries the page to come back to", () => {
    expect(signInHref("/dashboard")).toBe(
      "/auth/github/login?next=%2Fdashboard",
    );
    expect(signInHref("/p/paperplane/stash")).toBe(
      "/auth/github/login?next=%2Fp%2Fpaperplane%2Fstash",
    );
  });

  it("has one plain sentence for every reason the server sends back", () => {
    for (const reason of [
      "cancelled",
      "expired",
      "github",
      "server",
      "setup",
    ]) {
      const message = signInMessage(`?signin=${reason}`);
      expect(message, reason).toBeTruthy();
      expect(message, reason).not.toContain(
        reason === "github" ? "undefined" : "null",
      );
    }
    expect(signInMessage("?signin=cancelled")).toContain("cancelled");
  });

  it("says nothing without a reason, and something sensible for an unknown one", () => {
    expect(signInMessage("")).toBeNull();
    expect(signInMessage("?other=1")).toBeNull();
    expect(signInMessage("?signin=<script>")).toBe(
      "The sign-in didn't finish. Try again.",
    );
  });
});
