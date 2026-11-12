import { describe, expect, it, vi } from "vitest";

import {
  CHANNEL_LIST_NOT_READY,
  connectWithFreshToken,
  inTurn,
  openWhenReady,
  type ConnectOutcome,
} from "./connect";

const OK: ConnectOutcome = { ok: true };
const TOKEN_INCORRECT: ConnectOutcome = { ok: false, code: 31004 };
const TOKEN_EXPIRED: ConnectOutcome = { ok: false, code: 31020 };
const NETWORK_DOWN: ConnectOutcome = { ok: false, code: 30001 };

function setup(outcomes: readonly ConnectOutcome[]) {
  let call = 0;
  const fetchToken = vi.fn(async () => `token-${call + 1}`);
  const connect = vi.fn(async (token: string) => {
    void token;
    return outcomes[call++] ?? NETWORK_DOWN;
  });
  return { fetchToken, connect };
}

describe("FR-AUTH-3: connect, and on an invalid token fetch a new one once", () => {
  it("connects with the first token when it works", async () => {
    const deps = setup([OK]);
    await expect(connectWithFreshToken(deps)).resolves.toEqual(OK);
    expect(deps.fetchToken).toHaveBeenCalledTimes(1);
    expect(deps.connect).toHaveBeenCalledWith("token-1");
  });

  it("fetches a new token once when the first is refused, and uses it", async () => {
    for (const refusal of [TOKEN_INCORRECT, TOKEN_EXPIRED]) {
      const deps = setup([refusal, OK]);
      await expect(connectWithFreshToken(deps)).resolves.toEqual(OK);
      expect(deps.fetchToken).toHaveBeenCalledTimes(2);
      expect(deps.connect).toHaveBeenLastCalledWith("token-2");
    }
  });

  it("gives up after the second refusal, and says why", async () => {
    const deps = setup([TOKEN_INCORRECT, TOKEN_EXPIRED, OK]);
    await expect(connectWithFreshToken(deps)).resolves.toEqual(TOKEN_EXPIRED);
    expect(deps.fetchToken).toHaveBeenCalledTimes(2);
    expect(deps.connect).toHaveBeenCalledTimes(2);
  });

  it("does not ask for a new token when the failure is not about the token", async () => {
    const deps = setup([NETWORK_DOWN, OK]);
    await expect(connectWithFreshToken(deps)).resolves.toEqual(NETWORK_DOWN);
    expect(deps.fetchToken).toHaveBeenCalledTimes(1);
    expect(deps.connect).toHaveBeenCalledTimes(1);
  });

  it("lets a failed token request reach the caller", async () => {
    const deps = {
      fetchToken: vi.fn(async (): Promise<string> => {
        throw new Error("Sign in first.");
      }),
      connect: vi.fn(async () => OK),
    };
    await expect(connectWithFreshToken(deps)).rejects.toThrow("Sign in first.");
    expect(deps.connect).not.toHaveBeenCalled();
  });
});

describe("opening a room right after connecting", () => {
  const READY = { isOk: true, code: 0 };
  const NOT_YET = { isOk: false, code: CHANNEL_LIST_NOT_READY };
  const REFUSED = { isOk: false, code: 22406 };

  function setupOpen(outcomes: readonly { isOk: boolean; code: number }[]) {
    let call = 0;
    const open = vi.fn(async () => outcomes[call++] ?? NOT_YET);
    const pause = vi.fn(async () => {});
    return { open, pause };
  }

  it("opens at once when Chat UI is ready", async () => {
    const { open, pause } = setupOpen([READY]);
    await expect(openWhenReady(open, pause)).resolves.toEqual(READY);
    expect(open).toHaveBeenCalledTimes(1);
    expect(pause).not.toHaveBeenCalled();
  });

  it("waits and tries again while the channel list is not ready", async () => {
    const { open, pause } = setupOpen([NOT_YET, NOT_YET, READY]);
    await expect(openWhenReady(open, pause)).resolves.toEqual(READY);
    expect(open).toHaveBeenCalledTimes(3);
    expect(pause).toHaveBeenCalledTimes(2);
  });

  it("gives up after its attempts and says what Chat UI last answered", async () => {
    const { open, pause } = setupOpen([]);
    await expect(openWhenReady(open, pause, 4)).resolves.toEqual(NOT_YET);
    expect(open).toHaveBeenCalledTimes(4);
  });

  it("stops trying once the page has been left", async () => {
    // The room was closed while Chat UI was still loading its channel list. Opening it later
    // would put its messages under the next room.
    const { open } = setupOpen([NOT_YET, NOT_YET, READY]);
    let hasLeft = false;
    const pause = vi.fn(async () => {
      hasLeft = true;
    });
    await expect(
      openWhenReady(open, pause, 20, () => hasLeft),
    ).resolves.toEqual(NOT_YET);
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("does not retry a real refusal", async () => {
    const { open, pause } = setupOpen([REFUSED, READY]);
    await expect(openWhenReady(open, pause)).resolves.toEqual(REFUSED);
    expect(open).toHaveBeenCalledTimes(1);
    expect(pause).not.toHaveBeenCalled();
  });
});

describe("one room is opened at a time", () => {
  function deferred() {
    let resolve = () => {};
    const promise = new Promise<void>((done) => {
      resolve = done;
    });
    return { promise, resolve };
  }

  it("starts the next task only when the one before it has ended", async () => {
    const log: string[] = [];
    const first = deferred();
    const a = inTurn(async () => {
      log.push("a starts");
      await first.promise;
      log.push("a ends");
      return "a";
    });
    const b = inTurn(async () => {
      log.push("b starts");
      return "b";
    });
    await Promise.resolve();
    expect(log).toEqual(["a starts"]);
    first.resolve();
    await expect(Promise.all([a, b])).resolves.toEqual(["a", "b"]);
    expect(log).toEqual(["a starts", "a ends", "b starts"]);
  });

  it("a task that fails does not hold up the next one, and its failure reaches its caller", async () => {
    const failed = inTurn(async (): Promise<string> => {
      throw new Error("could not open");
    });
    const next = inTurn(async () => "opened");
    await expect(failed).rejects.toThrow("could not open");
    await expect(next).resolves.toBe("opened");
  });
});
