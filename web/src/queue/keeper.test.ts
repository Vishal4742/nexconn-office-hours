import { describe, expect, it } from "vitest";

import { makeClock } from "./clock";
import { liveSession, queues } from "./fixtures";
import { keepQueue } from "./keeper";
import type { Ping } from "./ping";
import type { Queue } from "./types";

/** In these tests the page's clock runs 5 s behind the server's. */
const BEHIND = 5_000;
const serverSays = (pageTime: number) =>
  new Date(pageTime + BEHIND).toISOString();

/** The live session's queue at one version, answered when the page's clock said `pageTime`. */
const at = (version: number, pageTime = 0): Queue => ({
  ...queues.liveWatching,
  session: { ...liveSession, version },
  serverTime: serverSays(pageTime),
});
const ping = (version: number, sentAt = 0, sessionId = "12"): Ping => ({
  sessionId,
  version,
  sentAt,
});
const versions = (shown: readonly Queue[]) =>
  shown.map((queue) => queue.session?.version);

/** A keeper whose fetches are answered by hand, on a clock that is moved by hand. */
function setUp() {
  const page = { now: 0 };
  const asked: Array<{
    readonly answer: (queue: Queue) => void;
    readonly fail: (failure: unknown) => void;
  }> = [];
  const shown: Queue[] = [];
  const failures: unknown[] = [];
  const latencies: number[] = [];
  const keeper = keepQueue({
    load: () =>
      new Promise<Queue>((resolve, reject) => {
        asked.push({ answer: resolve, fail: reject });
      }),
    clock: makeClock(() => page.now),
    onQueue: (queue) => shown.push(queue),
    onFailure: (failure) => failures.push(failure),
    onLatency: (ms) => latencies.push(ms),
  });
  return { keeper, page, asked, shown, failures, latencies };
}

/** Lets everything that waits on an answered fetch run. */
const turn = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

/** A keeper that already holds version 9. */
async function holding() {
  const all = setUp();
  all.keeper.refresh();
  all.asked[0].answer(at(9));
  await turn();
  return all;
}

describe("keepQueue: loading", () => {
  it("FR-QUEUE-6: asks the server and hands the queue over", async () => {
    const { keeper, asked, shown } = setUp();
    keeper.refresh();
    expect(asked).toHaveLength(1);
    expect(shown).toEqual([]);

    asked[0].answer(at(9));
    await turn();
    expect(versions(shown)).toEqual([9]);
  });

  it("plan decision 4: one fetch at a time, and triggers during a fetch ask for exactly one more", async () => {
    const { keeper, asked, shown } = setUp();
    keeper.refresh(); // the page opened
    keeper.refresh(); // the window got focus
    keeper.ping(ping(9)); // nothing is held yet, so any ping of this room asks
    keeper.refresh(); // the connection came up
    expect(asked).toHaveLength(1);

    asked[0].answer(at(9));
    await turn();
    expect(asked).toHaveLength(2);

    asked[1].answer(at(9));
    await turn();
    expect(asked).toHaveLength(2);
    expect(versions(shown)).toEqual([9, 9]);
  });

  it("says that a fetch failed only while there is nothing to show", async () => {
    const { keeper, asked, shown, failures } = setUp();
    keeper.refresh();
    asked[0].fail(new Error("offline"));
    await turn();
    expect(failures).toHaveLength(1);

    keeper.refresh();
    asked[1].answer(at(9));
    await turn();
    keeper.refresh();
    asked[2].fail(new Error("offline"));
    await turn();
    expect(failures).toHaveLength(1);
    expect(versions(shown)).toEqual([9]);
  });

  it("hands nothing over once it was stopped", async () => {
    const { keeper, asked, shown, failures } = setUp();
    keeper.refresh();
    keeper.stop();
    asked[0].answer(at(9));
    await turn();

    keeper.refresh();
    keeper.ping(ping(10));
    keeper.accept(at(10));
    expect(asked).toHaveLength(1);
    expect(shown).toEqual([]);
    expect(failures).toEqual([]);
  });

  it("stays quiet about a fetch that fails after it was stopped", async () => {
    const { keeper, asked, failures } = setUp();
    keeper.refresh();
    keeper.stop();
    asked[0].fail(new Error("offline"));
    await turn();
    expect(failures).toEqual([]);
  });
});

describe("keepQueue: pings", () => {
  it("FR-QUEUE-4: a newer ping asks for the queue, and only the server's answer is shown", async () => {
    const { keeper, asked, shown } = await holding();
    keeper.ping(ping(10));
    expect(asked).toHaveLength(2);
    expect(versions(shown)).toEqual([9]); // never acts on a ping alone

    asked[1].answer(at(10));
    await turn();
    expect(versions(shown)).toEqual([9, 10]);
  });

  it("FR-QUEUE-4: a ping that is not newer than what is held asks for nothing", async () => {
    const { keeper, asked } = await holding();
    keeper.ping(ping(9));
    keeper.ping(ping(8));
    keeper.ping(ping(99, 0, "11")); // an earlier session, replayed after a reconnect
    expect(asked).toHaveLength(1);
  });

  it("a ping of a later session asks, whatever its version", async () => {
    const { keeper, asked } = await holding();
    keeper.ping(ping(1, 0, "13"));
    expect(asked).toHaveLength(2);
  });
});

describe("keepQueue: an answer that lands late", () => {
  it("a fetch's answer older than what is held is dropped", async () => {
    const { keeper, asked, shown } = await holding();
    keeper.refresh(); // asked before this page's own action
    keeper.accept(at(11)); // the answer to that action arrives first
    asked[1].answer(at(10));
    await turn();
    expect(versions(shown)).toEqual([9, 11]);
  });

  it("an action's answer older than what is held is dropped", async () => {
    const { keeper, shown } = await holding();
    keeper.accept(at(11));
    keeper.accept(at(10));
    expect(versions(shown)).toEqual([9, 11]);
  });

  it("at the same version the later answer of the server wins, whichever lands last", async () => {
    const { keeper, shown } = await holding();
    const planned = { ...at(9, 2_000), nextSessionAt: "2026-11-19T11:30:00Z" };
    keeper.accept(planned); // the next session's time was set: no new version
    keeper.accept(at(9, 1_000)); // answered before that, landing after it
    expect(shown).toHaveLength(2);
    expect(shown[1]).toBe(planned);
  });

  it("takes the queue an action answered with before any fetch was answered", () => {
    const { keeper, shown } = setUp();
    keeper.accept(at(3));
    expect(versions(shown)).toEqual([3]);
  });
});

describe("keepQueue: NFR-PERF-1, the latency of a ping on one clock", () => {
  it("counts from the ping's sentAt on the server's clock to the queue being held, on the page's clock corrected", async () => {
    const { keeper, page, asked, latencies } = setUp();
    page.now = 1_000;
    keeper.refresh();
    page.now = 1_200;
    asked[0].answer(at(9, 1_100)); // the middle of the fetch: the clocks are 5,000 apart
    await turn();

    // The server changes the queue at its 6,500. The ping reaches the page 150 ms later.
    page.now = 1_650;
    keeper.ping(ping(10, 6_500));
    page.now = 1_900;
    asked[1].answer(at(10, 1_775));
    await turn();
    expect(latencies).toEqual([400]); // (1,900 + 5,000) - 6,500
  });

  it("a ping during a fetch is counted when the queue it tells of is held", async () => {
    const { keeper, page, asked, latencies } = await holding();
    page.now = 2_000;
    keeper.ping(ping(10, BEHIND + 1_900));
    page.now = 2_100;
    keeper.ping(ping(11, BEHIND + 2_050)); // during the fetch: one more afterwards
    page.now = 2_200;
    asked[1].answer(at(10, 2_100)); // this answer does not know version 11 yet
    await turn();
    expect(latencies).toEqual([300]);
    expect(asked).toHaveLength(3);

    page.now = 2_400;
    asked[2].answer(at(11, 2_300));
    await turn();
    expect(latencies).toEqual([300, 350]);
  });

  it("one answer that covers two pings counts both", async () => {
    const { keeper, page, asked, latencies } = await holding();
    page.now = 2_000;
    keeper.ping(ping(10, BEHIND + 1_900));
    keeper.ping(ping(11, BEHIND + 1_950));
    page.now = 2_200;
    asked[1].answer(at(11, 2_100));
    await turn();
    expect(latencies).toEqual([300, 250]);
  });

  it("counts when this page's own action brought the queue first", async () => {
    const { keeper, page, asked, latencies } = await holding();
    page.now = 2_000;
    keeper.ping(ping(10, BEHIND + 1_900));
    page.now = 2_050;
    keeper.accept(at(10, 2_040));
    expect(latencies).toEqual([150]);

    page.now = 2_300;
    asked[1].answer(at(10, 2_150));
    await turn();
    expect(latencies).toEqual([150]); // counted once
  });

  it("does not count a ping while nothing is held: it may be a replay of the past", async () => {
    const { keeper, asked, latencies } = setUp();
    keeper.refresh();
    keeper.ping(ping(3, 12));
    asked[0].answer(at(9));
    await turn();
    asked[1].answer(at(9));
    await turn();
    expect(latencies).toEqual([]);
  });

  it("does not count a ping whose fetch failed", async () => {
    const { keeper, asked, latencies } = await holding();
    keeper.ping(ping(10, BEHIND));
    asked[1].fail(new Error("offline"));
    await turn();
    keeper.refresh();
    asked[2].answer(at(10));
    await turn();
    expect(latencies).toEqual([]);
  });

  it("does not count a ping that was already old when it arrived: a replay after a reconnect", async () => {
    const { keeper, page, asked, latencies } = await holding();
    page.now = 120_000;
    keeper.ping(ping(10, BEHIND + 60_000)); // sent a minute before it arrived
    expect(asked).toHaveLength(2); // it still asks for the queue

    asked[1].answer(at(10, 120_000));
    await turn();
    expect(latencies).toEqual([]);
  });

  it("is never below zero when the clocks were read a little off", async () => {
    const { keeper, page, asked, latencies } = await holding();
    page.now = 2_000;
    keeper.ping(ping(10, BEHIND + 2_500)); // sent "after" it arrived
    asked[1].answer(at(10, 2_000));
    await turn();
    expect(latencies).toEqual([0]);
  });
});
