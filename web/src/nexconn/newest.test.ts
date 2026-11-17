import { describe, expect, it } from "vitest";

import { newestAnswer } from "./newest";

/** A load whose calls are answered by hand, in any order. */
function setUp() {
  const calls: Array<{
    readonly answer: (value: string) => void;
    readonly fail: (failure: unknown) => void;
  }> = [];
  const used: string[] = [];
  const ask = newestAnswer(
    () =>
      new Promise<string>((resolve, reject) => {
        calls.push({ answer: resolve, fail: reject });
      }),
    (value) => used.push(value),
  );
  return { ask, calls, used };
}

describe("newestAnswer (state-machine audit, bug 13)", () => {
  it("uses each answer while they come in order", async () => {
    const { ask, calls, used } = setUp();
    const first = ask();
    calls[0].answer("ana");
    await first;
    const second = ask();
    calls[1].answer("ana, ben");
    await second;
    expect(used).toEqual(["ana", "ana, ben"]);
  });

  it("drops an answer that lands after the answer to a later call was used", async () => {
    const { ask, calls, used } = setUp();
    const older = ask();
    const newer = ask();
    calls[1].answer("ana, ben");
    await newer;
    calls[0].answer("ana");
    await older;
    expect(used).toEqual(["ana, ben"]);
  });

  it("still uses an older answer when the later call failed: the newcomers in it are news", async () => {
    const { ask, calls, used } = setUp();
    const older = ask();
    const newer = ask();
    calls[1].fail(new Error("offline"));
    await expect(newer).rejects.toThrow("offline");
    calls[0].answer("ana, ben");
    await older;
    expect(used).toEqual(["ana, ben"]);
  });
});
