import { describe, expect, it } from "vitest";

import { oneAtATime } from "./oneAtATime";

/** A task whose runs are finished by hand, one by one. */
function setUp() {
  const runs: Array<{
    readonly finish: () => void;
    readonly fail: () => void;
  }> = [];
  const trigger = oneAtATime(
    () =>
      new Promise<void>((resolve, reject) => {
        runs.push({ finish: resolve, fail: () => reject(new Error("down")) });
      }),
  );
  return { runs, trigger };
}

/** Lets everything that waits on a finished promise run. */
const turn = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

describe("oneAtATime", () => {
  it("runs at once when nothing is running", () => {
    const { runs, trigger } = setUp();
    trigger();
    expect(runs).toHaveLength(1);
  });

  it("plan decision 4: calls during a run ask for exactly one more afterwards", async () => {
    const { runs, trigger } = setUp();
    trigger();
    trigger();
    trigger();
    trigger();
    expect(runs).toHaveLength(1);

    runs[0].finish();
    await turn();
    expect(runs).toHaveLength(2);

    runs[1].finish();
    await turn();
    expect(runs).toHaveLength(2);
  });

  it("a call during the extra run asks for one more again", async () => {
    const { runs, trigger } = setUp();
    trigger();
    trigger();
    runs[0].finish();
    await turn();
    trigger();
    runs[1].finish();
    await turn();
    expect(runs).toHaveLength(3);
  });

  it("runs again when it is called after everything finished", async () => {
    const { runs, trigger } = setUp();
    trigger();
    runs[0].finish();
    await turn();
    trigger();
    expect(runs).toHaveLength(2);
  });

  it("a run that fails does not block the next one", async () => {
    const { runs, trigger } = setUp();
    trigger();
    trigger();
    runs[0].fail();
    await turn();
    expect(runs).toHaveLength(2);

    runs[1].fail();
    await turn();
    trigger();
    expect(runs).toHaveLength(3);
  });

  it("a task that throws before it returns a promise does not block the next one", async () => {
    let calls = 0;
    const trigger = oneAtATime(() => {
      calls++;
      throw new Error("broken");
    });
    trigger();
    await turn();
    trigger();
    expect(calls).toBe(2);
  });
});
