import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, REQUEST_TIMEOUT_MS, api, refusalOf } from "./api";

function reply(status: number, body: string, type = "application/json") {
  return new Response(body, { status, headers: { "Content-Type": type } });
}

function stubFetch(result: Response | Error) {
  const fetchMock = vi.fn(async () => {
    if (result instanceof Error) throw result;
    return result.clone(); // a Response body can be read once; each call gets its own
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("api", () => {
  it("returns the data of an ok envelope", async () => {
    stubFetch(reply(200, `{"ok":true,"data":{"login":"mia-k"}}`));
    await expect(api.get<{ login: string }>("/api/me")).resolves.toEqual({
      login: "mia-k",
    });
  });

  it("NFR-SEC-5: every POST is JSON, {} when it has nothing to say", async () => {
    const fetchMock = stubFetch(reply(200, `{"ok":true}`));
    await api.post("/auth/logout");
    await api.post("/api/projects", { owner: "paperplane", repo: "stash" });
    const [first, second] = fetchMock.mock.calls.map(
      (call) => (call as unknown as [string, RequestInit])[1],
    );
    expect(first.method).toBe("POST");
    expect(first.body).toBe("{}");
    expect(new Headers(first.headers).get("Content-Type")).toBe(
      "application/json",
    );
    expect(second.body).toBe(`{"owner":"paperplane","repo":"stash"}`);
  });

  it("turns a refusal into an ApiError with the server's own sentence", async () => {
    stubFetch(reply(401, `{"ok":false,"error":"Sign in first."}`));
    const failure = await api.get("/api/me").catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(401);
    expect((failure as ApiError).message).toBe("Sign in first.");
  });

  it("never shows a platform error page or a parse error to the user", async () => {
    stubFetch(reply(502, "<html><h1>502 Bad Gateway</h1></html>", "text/html"));
    const failure = await api.get("/api/me").catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(502);
    expect((failure as ApiError).message).toBe(
      "The server had a problem (502). Try again in a moment.",
    );
  });

  it("treats a 200 that is not our envelope as a failure", async () => {
    stubFetch(
      reply(200, "<!doctype html><title>Office Hours</title>", "text/html"),
    );
    await expect(api.get("/api/me")).rejects.toBeInstanceOf(ApiError);
    stubFetch(reply(200, `{"ok":false,"error":"Nope"}`));
    await expect(api.get("/api/me")).rejects.toThrow("Nope");
  });

  it("says so when the server can't be reached", async () => {
    stubFetch(new TypeError("Failed to fetch"));
    const failure = await api.get("/api/me").catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(0);
    expect((failure as ApiError).message).toBe(
      "Can't reach the server. Check your connection and try again.",
    );
  });
});

describe("api: a server that never answers (state-machine audit, bug 11)", () => {
  const aborted = () =>
    new DOMException("The request was aborted.", "AbortError");

  afterEach(() => vi.useRealTimers());

  /** Gives up once the timeout passes, and answers with what the call threw. */
  async function failureAfterTimeout(call: Promise<unknown>) {
    const failure = call.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);
    return failure;
  }

  it("gives up on a request that gets no answer at all, and says so", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_path: string, init: RequestInit) =>
          new Promise((_resolve, reject) =>
            init.signal?.addEventListener("abort", () => reject(aborted())),
          ),
      ),
    );
    const failure = await failureAfterTimeout(api.get("/api/p/o/r/queue"));
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).status).toBe(0);
    expect((failure as ApiError).message).toBe(
      "The server took too long to answer. Try again.",
    );
  });

  it("gives up on an answer whose body stops arriving, instead of calling it a refusal", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path: string, init: RequestInit) => {
        const body = new ReadableStream({
          start(stream) {
            init.signal?.addEventListener("abort", () =>
              stream.error(aborted()),
            );
          },
        });
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const failure = await failureAfterTimeout(api.post("/api/hands/7/plus"));
    expect(failure).toBeInstanceOf(ApiError);
    expect((failure as ApiError).message).toBe(
      "The server took too long to answer. Try again.",
    );
  });

  it("leaves no timer behind once an answer was read", async () => {
    vi.useFakeTimers();
    stubFetch(reply(200, `{"ok":true,"data":1}`));
    await expect(api.get<number>("/api/stats")).resolves.toBe(1);
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("refusalOf", () => {
  it("keeps the server's sentence, and knows when signing in again is the cure", () => {
    expect(refusalOf(new ApiError(403, "Join this room first."))).toEqual({
      message: "Join this room first.",
      needsSignIn: false,
    });
    // Our own session ran out, or GitHub stopped accepting the token inside it.
    expect(refusalOf(new ApiError(401, "Sign in first."))).toEqual({
      message: "Sign in first.",
      needsSignIn: true,
    });
    expect(refusalOf("not an error").needsSignIn).toBe(false);
    expect(refusalOf("not an error").message.length).toBeGreaterThan(0);
  });
});
