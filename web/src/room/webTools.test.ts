import { afterEach, describe, expect, it, vi } from "vitest";

import type { QueueState } from "../queue/store";
import type { Hand, Queue } from "../queue/types";
import { modelContextOf, offerRoomTools, roomTools } from "./webTools";
import type { WebTool } from "./webTools";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn(
    async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const person = {
  userId: "gh3",
  login: "mia",
  name: "Mia",
  avatarUrl: "https://avatars.example/3",
};

const waiting: Hand = {
  id: "31",
  title: "Where is the cache stored?",
  details: "",
  status: "waiting",
  author: person,
  raisedAt: "2026-11-14T03:00:00Z",
  takenAt: null,
  takenBy: null,
  ackedAt: null,
  closedAt: null,
  awayCount: 0,
  plusCount: 2,
  plusByMe: false,
  isMine: false,
  faqId: "",
  faqSlug: "",
};

const queue: Queue = {
  session: {
    id: "12",
    number: 3,
    title: "Thursday office hours",
    live: true,
    handsClosed: false,
    startedAt: "2026-11-14T02:30:00Z",
    endedAt: null,
    plannedMinutes: 45,
    version: 4,
    host: person,
  },
  hands: [waiting],
  done: [],
  nextSessionAt: null,
  medianAnswerSeconds: 0,
  serverTime: "2026-11-14T03:05:00Z",
  removedHandIds: [],
};

const ready =
  (q: Queue): (() => QueueState) =>
  () => ({
    status: "ready",
    queue: q,
  });
const stash = { owner: "PaperPlane", repo: "Stash" };
/** act as the room does it, minus the store: the call itself. */
const act = async (call: () => Promise<Queue>) => {
  await call();
};

function tool(tools: readonly WebTool[], name: string): WebTool {
  const found = tools.find((t) => t.name === name);
  if (!found) throw new Error(`no tool ${name}`);
  return found;
}

const textOf = (r: { content: readonly { text: string }[] }) =>
  r.content.map((c) => c.text).join("\n");

describe("modelContextOf (FR-AGENT-5)", () => {
  const mc = { registerTool: () => undefined };
  it("takes document.modelContext, else the older navigator one, else none", () => {
    expect(modelContextOf({ modelContext: mc }, {})).toBe(mc);
    expect(modelContextOf({}, { modelContext: mc })).toBe(mc);
    expect(modelContextOf({}, {})).toBeNull();
    expect(modelContextOf({ modelContext: {} }, {})).toBeNull();
  });
});

describe("roomTools (FR-AGENT-5)", () => {
  it("offers a member the member's actions, and a maintainer the room's too", () => {
    const names = (isMaintainer: boolean) =>
      roomTools(stash, isMaintainer, ready(queue), act).map((t) => t.name);
    expect(names(false)).toEqual([
      "get_queue",
      "similar_answers",
      "raise_hand",
      "withdraw_hand",
      "plus_one",
      "im_here",
    ]);
    expect(names(true)).toEqual([
      ...names(false),
      "next_hand",
      "skip_hand",
      "mark_away",
      "merge_hand",
      "answer_hand",
      "remove_hand",
      "last_call",
      "end_session",
    ]);
  });

  it("marks what is consequential, and what carries people's words", () => {
    const tools = roomTools(stash, true, ready(queue), act);
    const consequential = tools
      .filter((t) => t.annotations.consequentialHint)
      .map((t) => t.name);
    expect(consequential).toEqual([
      "withdraw_hand",
      "merge_hand",
      "answer_hand",
      "remove_hand",
      "end_session",
    ]);
    expect(tool(tools, "get_queue").annotations).toEqual({
      readOnlyHint: true,
      untrustedContentHint: true,
    });
  });

  it("get_queue tells the session and each open hand with its id", async () => {
    const result = await tool(
      roomTools(stash, false, ready(queue), act),
      "get_queue",
    ).execute({});
    const seen = JSON.parse(textOf(result)) as {
      session: { id: string; live: boolean };
      hands: { id: string; title: string; author: string }[];
    };
    expect(seen.session).toMatchObject({ id: "12", live: true });
    expect(seen.hands).toEqual([
      expect.objectContaining({
        id: "31",
        title: "Where is the cache stored?",
        author: "@mia",
        plusCount: 2,
      }),
    ]);
  });

  it("raise_hand raises it in the live session, as the sheet does", async () => {
    const fetchMock = stubFetch({ ok: true, data: queue });
    const result = await tool(
      roomTools(stash, false, ready(queue), act),
      "raise_hand",
    ).execute({ title: "How do I pin the cache?", details: "I tried X." });
    expect(result.isError).toBeFalsy();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/sessions/12/hands",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          title: "How do I pin the cache?",
          details: "I tried X.",
        }),
      }),
    );
  });

  it("says so when there is no live session, and asks nothing of the server", async () => {
    const fetchMock = stubFetch({ ok: true, data: queue });
    const ended = { ...queue, session: { ...queue.session!, live: false } };
    const result = await tool(
      roomTools(stash, true, ready(ended), act),
      "next_hand",
    ).execute({});
    expect(result.isError).toBe(true);
    expect(textOf(result)).toContain("No session is live");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses a hand id that isn't one, before any request", async () => {
    const fetchMock = stubFetch({ ok: true, data: queue });
    for (const handId of ["31/remove", "", 31.5, null]) {
      const result = await tool(
        roomTools(stash, false, ready(queue), act),
        "withdraw_hand",
      ).execute({ hand_id: handId });
      expect(result.isError).toBe(true);
      expect(textOf(result)).toContain("hand_id");
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("passes the server's refusal on, word for word", async () => {
    stubFetch(
      { ok: false, error: "Only a maintainer of this project can do that." },
      403,
    );
    const result = await tool(
      roomTools(stash, true, ready(queue), act),
      "skip_hand",
    ).execute({ hand_id: 31 });
    expect(result.isError).toBe(true);
    expect(textOf(result)).toBe(
      "Only a maintainer of this project can do that.",
    );
  });
});

describe("offerRoomTools (FR-AGENT-5)", () => {
  it("registers each tool, and unregisters them all when the room closes", () => {
    const signals: AbortSignal[] = [];
    const registerTool = vi.fn(
      (_tool: WebTool, options: { signal: AbortSignal }) => {
        signals.push(options.signal);
      },
    );
    const tools = roomTools(stash, false, ready(queue), act);
    const close = offerRoomTools({ registerTool }, tools);
    expect(registerTool).toHaveBeenCalledTimes(tools.length);
    expect(signals.every((s) => !s.aborted)).toBe(true);
    close();
    expect(signals.every((s) => s.aborted)).toBe(true);
  });

  it("does nothing without WebMCP, and stays quiet when the page may not register", async () => {
    expect(() => offerRoomTools(null, [])()).not.toThrow();
    const refused = vi.fn(async () => {
      throw new DOMException("tools are off here", "NotAllowedError");
    });
    expect(() =>
      offerRoomTools(
        { registerTool: refused },
        roomTools(stash, false, ready(queue), act),
      ),
    ).not.toThrow();
    await Promise.resolve();
  });
});
