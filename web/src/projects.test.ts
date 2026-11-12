import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "./api";
import {
  joinProject,
  listMembers,
  listMyProjects,
  loadProject,
  parseRepo,
  registerProject,
  roomPath,
} from "./projects";

function stubFetch(status: number, body: string) {
  const fetchMock = vi.fn(
    async () =>
      new Response(body, {
        status,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

const stash = {
  groupId: "pabcdefghijklmnopqrstuvwx23",
  owner: "PaperPlane",
  repo: "Stash",
  description: "A tiny HTTP cache.",
  isMember: true,
  isMaintainer: true,
};

describe("parseRepo", () => {
  it.each([
    ["paperplane/stash", "paperplane", "stash"],
    ["  paperplane/stash \n", "paperplane", "stash"],
    ["PaperPlane/Stash.js", "PaperPlane", "Stash.js"],
    ["https://github.com/paperplane/stash", "paperplane", "stash"],
    ["https://github.com/paperplane/stash/", "paperplane", "stash"],
    ["http://www.github.com/paperplane/stash", "paperplane", "stash"],
    ["github.com/paperplane/stash", "paperplane", "stash"],
    ["https://github.com/paperplane/stash.git", "paperplane", "stash"],
    ["git@github.com:paperplane/stash.git", "paperplane", "stash"],
    ["https://github.com/paperplane/stash/issues/12", "paperplane", "stash"],
    [
      "https://github.com/paperplane/stash?tab=readme-ov-file#usage",
      "paperplane",
      "stash",
    ],
  ])("reads %j", (input, owner, repo) => {
    expect(parseRepo(input)).toEqual({ owner, repo });
  });

  it.each([
    "",
    "   ",
    "paperplane",
    "paperplane/",
    "/stash",
    "paperplane/stash/extra",
    "paper_plane/stash",
    "paperplane/sta sh",
    "paperplane/..",
    "paperplane/.",
    "paperplane/<b>",
    `${"a".repeat(40)}/stash`,
    `paperplane/${"r".repeat(101)}`,
    "https://github.com/paperplane",
    "https://gitlab.com/paperplane/stash",
    "https://github.com.evil.example/paperplane/stash",
    "https://evil.example/github.com/paperplane/stash",
  ])("NFR-SEC-3: refuses %j", (input) => {
    expect(parseRepo(input)).toBeNull();
  });
});

describe("roomPath", () => {
  it("FR-PROJ-3: a project's room lives at /p/owner/repo, in GitHub's spelling", () => {
    expect(roomPath({ owner: "PaperPlane", repo: "Stash.js" })).toBe(
      "/p/PaperPlane/Stash.js",
    );
  });
});

describe("projects API", () => {
  it("FR-PROJ-1: registering posts the owner and the repo as JSON", async () => {
    const fetchMock = stubFetch(201, JSON.stringify({ ok: true, data: stash }));
    await expect(
      registerProject({ owner: "paperplane", repo: "stash" }),
    ).resolves.toEqual(stash);
    const [path, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(path).toBe("/api/projects");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(`{"owner":"paperplane","repo":"stash"}`);
  });

  it("FR-PROJ-1: a refusal keeps the server's sentence and its status", async () => {
    stubFetch(
      403,
      `{"ok":false,"error":"GitHub says you don't have admin or maintain rights on mia-k/stash."}`,
    );
    const failure = await registerProject({
      owner: "mia-k",
      repo: "stash",
    }).catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(403);
    expect((failure as ApiError).message).toContain("admin or maintain rights");
  });

  it("lists my projects, and an empty list is a list", async () => {
    stubFetch(200, JSON.stringify({ ok: true, data: [stash] }));
    await expect(listMyProjects()).resolves.toEqual([stash]);
    stubFetch(200, `{"ok":true,"data":[]}`);
    await expect(listMyProjects()).resolves.toEqual([]);
  });

  it("a reply that is not what we expect is an error a person can read", async () => {
    for (const data of [
      `null`,
      `{"groupId":"x"}`,
      `[{"groupId":"x","owner":"a"}]`,
      `[{"groupId":1,"owner":"a","repo":"b","description":"","isMember":true,"isMaintainer":true}]`,
    ]) {
      stubFetch(200, `{"ok":true,"data":${data}}`);
      const failure = await listMyProjects().catch((error: unknown) => error);
      expect(failure, data).toBeInstanceOf(ApiError);
      expect((failure as ApiError).message, data).toMatch(/unexpected/);
    }
    stubFetch(201, `{"ok":true,"data":{"owner":"a","repo":"b"}}`);
    await expect(
      registerProject({ owner: "a", repo: "b" }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});

describe("the room's API", () => {
  const name = { owner: "PaperPlane", repo: "Stash.js" };
  const visitorView = {
    owner: "PaperPlane",
    repo: "Stash.js",
    description: "",
    isMember: false,
    isMaintainer: false,
  };

  it("FR-PROJ-3: reads a project's page data; a visitor is not told the chat room", async () => {
    const fetchMock = stubFetch(
      200,
      JSON.stringify({ ok: true, data: visitorView }),
    );
    await expect(loadProject(name)).resolves.toEqual(visitorView);
    expect(fetchMock.mock.calls[0]).toEqual([
      "/api/p/PaperPlane/Stash.js",
      { method: "GET" },
    ]);
  });

  it("FR-PROJ-4: joining is a JSON POST and answers with the project as a member sees it", async () => {
    const memberView = { ...visitorView, isMember: true, groupId: "pabc" };
    const fetchMock = stubFetch(
      200,
      JSON.stringify({ ok: true, data: memberView }),
    );
    await expect(joinProject(name)).resolves.toEqual(memberView);
    const [path, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(path).toBe("/api/p/PaperPlane/Stash.js/join");
    expect(init.method).toBe("POST");
    expect(init.body).toBe("{}");
  });

  it("FR-PROJ-4: lists the room's people for the chat's names and avatars", async () => {
    const people = [
      { userId: "gh42", name: "Mia K", avatarUrl: "https://a.example/42" },
      { userId: "gh7", name: "quiet-dev", avatarUrl: "" },
    ];
    const fetchMock = stubFetch(
      200,
      JSON.stringify({ ok: true, data: people }),
    );
    await expect(listMembers(name)).resolves.toEqual(people);
    expect(fetchMock.mock.calls[0]).toEqual([
      "/api/p/PaperPlane/Stash.js/members",
      { method: "GET" },
    ]);
  });

  it("a member list that is not one is an error a person can read", async () => {
    for (const data of [
      `null`,
      `{}`,
      `[{"userId":"gh42"}]`,
      `[{"userId":"gh42","name":7,"avatarUrl":""}]`,
      `[{"userId":"gh42","name":"","avatarUrl":""}]`,
    ]) {
      stubFetch(200, `{"ok":true,"data":${data}}`);
      const failure = await listMembers(name).catch((error: unknown) => error);
      expect(failure, data).toBeInstanceOf(ApiError);
      expect((failure as ApiError).message, data).toMatch(/unexpected/);
    }
  });
});
