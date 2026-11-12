import { describe, expect, it } from "vitest";

import { matchRoute, titleFor } from "./router";

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
      "/Dashboard",
      "//evil.example",
    ]) {
      expect(matchRoute(path), path).toEqual({ name: "notFound" });
    }
  });
});

describe("titleFor", () => {
  it("NFR-A11Y-1: every page has its own title, so a change of page can be heard and found among tabs", () => {
    expect(titleFor({ name: "landing" })).toBe("Office Hours");
    expect(titleFor({ name: "dashboard" })).toBe(
      "Your projects · Office Hours",
    );
    expect(titleFor({ name: "room", owner: "PaperPlane", repo: "Stash" })).toBe(
      "PaperPlane/Stash · Office Hours",
    );
    expect(titleFor({ name: "notFound" })).toBe(
      "Page not found · Office Hours",
    );
  });
});
