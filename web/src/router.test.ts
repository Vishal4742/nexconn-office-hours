import { describe, expect, it } from "vitest";

import { matchRoute } from "./router";

describe("matchRoute", () => {
  it("knows the pages the web app owns", () => {
    expect(matchRoute("/")).toEqual({ name: "landing" });
    expect(matchRoute("/dashboard")).toEqual({ name: "dashboard" });
  });

  it("FR-PROJ-3: /p/owner/repo is the project's page, in GitHub's own spelling", () => {
    expect(matchRoute("/p/paperplane/stash")).toEqual({
      name: "room",
      owner: "paperplane",
      repo: "stash",
    });
    expect(matchRoute("/p/Paper-Plane/My.Repo_1")).toEqual({
      name: "room",
      owner: "Paper-Plane",
      repo: "My.Repo_1",
    });
  });

  it("forgives one trailing slash", () => {
    expect(matchRoute("/dashboard/")).toEqual({ name: "dashboard" });
    expect(matchRoute("/p/paperplane/stash/")).toEqual({
      name: "room",
      owner: "paperplane",
      repo: "stash",
    });
  });

  it("NFR-SEC-3: anything that is not a real owner/repo is not found", () => {
    for (const path of [
      "/p",
      "/p/paperplane",
      "/p/paperplane/stash/extra",
      "/p/under_score/stash",
      "/p/paperplane/sta sh",
      "/p/paperplane/..",
      "/p/paperplane/.",
      `/p/${"a".repeat(40)}/stash`,
      `/p/paperplane/${"r".repeat(101)}`,
      "/nope",
      "/spike", // the Phase 0 test page is gone
      "/Dashboard",
      "//evil.example",
    ]) {
      expect(matchRoute(path), path).toEqual({ name: "notFound" });
    }
  });
});
