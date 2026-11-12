import { describe, expect, it } from "vitest";

import { issueUrl } from "./issueLink";

const repo = { owner: "PaperPlane", repo: "Stash" };

describe("issueUrl", () => {
  it("FR-FAQ-5: opens GitHub's new-issue page of the repo with the title and body filled in", () => {
    const url = new URL(
      issueUrl(repo, "How do I retry only on 5xx?", "Asked in office hours."),
    );
    expect(url.origin + url.pathname).toBe(
      "https://github.com/PaperPlane/Stash/issues/new",
    );
    expect(url.searchParams.get("title")).toBe("How do I retry only on 5xx?");
    expect(url.searchParams.get("body")).toBe("Asked in office hours.");
  });

  it("keeps text that could break an address inside its own field", () => {
    const title = "a&body=evil #frag?x=1 ünïcode\nline";
    const url = new URL(issueUrl(repo, title, "100% & more"));
    expect(url.searchParams.get("title")).toBe(title);
    expect(url.searchParams.get("body")).toBe("100% & more");
    expect(url.hash).toBe("");
    expect([...url.searchParams.keys()]).toEqual(["title", "body"]);
  });

  it("cuts a body that would make the address too long for GitHub", () => {
    const body = new URL(
      issueUrl(repo, "t", "x".repeat(9000)),
    ).searchParams.get("body");
    expect(body).toHaveLength(4000);
    expect(body?.endsWith("…")).toBe(true);
  });
});
