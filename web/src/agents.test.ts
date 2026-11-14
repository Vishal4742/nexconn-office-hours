import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "./api";
import { disconnectAgent, lastUsedLabel, listAgents } from "./agents";

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

const claude = {
  id: "G7ZJ3QK2",
  app: "Claude Code",
  returnsTo: "127.0.0.1",
  connectedAt: "2026-11-14T03:00:00Z",
  lastUsedAt: "2026-11-14T03:20:00Z",
};

describe("listAgents (FR-AGENT-3)", () => {
  it("reads the person's connected apps, a never-used one too", async () => {
    const fresh = { ...claude, id: "Q2", lastUsedAt: null };
    const fetchMock = stubFetch(
      200,
      JSON.stringify({ ok: true, data: [claude, fresh] }),
    );
    await expect(listAgents()).resolves.toEqual([claude, fresh]);
    expect(fetchMock).toHaveBeenCalledWith("/api/me/agents", {
      method: "GET",
    });
  });

  it("refuses a shape it doesn't know, instead of showing half of it", async () => {
    for (const data of [
      [{ ...claude, app: 7 }],
      [{ ...claude, lastUsedAt: undefined }],
      [{ id: "x" }],
      { agents: [] },
    ]) {
      stubFetch(200, JSON.stringify({ ok: true, data }));
      await expect(listAgents()).rejects.toBeInstanceOf(ApiError);
    }
  });
});

describe("disconnectAgent (FR-AGENT-3)", () => {
  it("posts to the agent's own address, its id escaped", async () => {
    const fetchMock = stubFetch(200, JSON.stringify({ ok: true }));
    await disconnectAgent("a/b?c");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/me/agents/a%2Fb%3Fc/disconnect",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("passes the server's sentence on", async () => {
    stubFetch(
      404,
      JSON.stringify({ ok: false, error: "There is no such connected app." }),
    );
    await expect(disconnectAgent("x")).rejects.toThrow(
      "There is no such connected app.",
    );
  });
});

describe("lastUsedLabel", () => {
  const now = Date.parse("2026-11-14T03:30:00Z");
  it.each([
    [null, "Not used yet"],
    ["2026-11-14T03:29:40Z", "Used just now"],
    ["2026-11-14T03:20:00Z", "Used 10 min ago"],
    ["2026-11-14T00:30:00Z", "Used 3 h ago"],
    ["2026-11-11T03:30:00Z", "Used 3 days ago"],
    ["2026-11-13T03:30:00Z", "Used 1 day ago"],
  ])("%s → %s", (lastUsedAt, label) => {
    expect(lastUsedLabel(lastUsedAt, now)).toBe(label);
  });
});
