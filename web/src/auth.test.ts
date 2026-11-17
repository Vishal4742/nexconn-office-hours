import { describe, expect, it } from "vitest";

import { mustReload, signInHref, signInMessage, signInRetryPath } from "./auth";
import type { Auth } from "./auth";

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

describe("signInRetryPath (state-machine audit, bug 10)", () => {
  it("tries again toward the page the failed sign-in started from", () => {
    expect(
      signInRetryPath("?signin=cancelled&next=%2Fp%2Fpaperplane%2Fstash"),
    ).toBe("/p/paperplane/stash");
    // The agent consent page comes with its own query, escaped as one value.
    expect(
      signInRetryPath(
        "?signin=github&next=%2Foauth%2Fauthorize%3Fclient_id%3Dabc%26state%3Dxyz",
      ),
    ).toBe("/oauth/authorize?client_id=abc&state=xyz");
  });

  it("falls back to the dashboard without a page of this site", () => {
    expect(signInRetryPath("?signin=expired")).toBe("/dashboard");
    expect(signInRetryPath("?signin=cancelled&next=")).toBe("/dashboard");
    expect(
      signInRetryPath("?signin=cancelled&next=https%3A%2F%2Fevil.example"),
    ).toBe("/dashboard");
    expect(signInRetryPath("?signin=cancelled&next=%2F%2Fevil.example")).toBe(
      "/dashboard",
    );
    expect(signInRetryPath("?signin=cancelled&next=%2F%5Cevil.example")).toBe(
      "/dashboard",
    );
  });
});

describe("mustReload (state-machine audit, bug 5)", () => {
  const signedInAs = (id: string): Auth => ({
    status: "signedIn",
    me: { id, login: id, name: id, avatarUrl: "" },
  });
  const cases: ReadonlyArray<readonly [string, Auth, string | null, boolean]> =
    [
      ["the same person", signedInAs("gh1"), "gh1", false],
      ["another person signed in elsewhere", signedInAs("gh1"), "gh2", true],
      ["signed out elsewhere", signedInAs("gh1"), null, true],
      [
        "signed in elsewhere while this tab was out",
        { status: "signedOut" },
        "gh1",
        true,
      ],
      ["still signed out", { status: "signedOut" }, null, false],
      ["this tab is still finding out", { status: "loading" }, "gh1", false],
      [
        "this tab could not find out",
        { status: "unreachable", message: "Can't reach the server." },
        null,
        false,
      ],
    ];

  it.each(cases)("%s", (_case, mine, theirs, expected) => {
    expect(mustReload(mine, theirs)).toBe(expected);
  });
});
