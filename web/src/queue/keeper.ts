// What keeps one room's queue current, with no browser or SDK in it: store.ts gives it the fetch,
// the clock and the triggers, and takes what it hands over.
import type { Clock } from "./clock";
import { oneAtATime } from "./oneAtATime";
import { isNewerPing, isOlderQueue } from "./ping";
import type { Ping } from "./ping";
import type { Queue } from "./types";

export type Keeper = {
  /** Asks the server for the queue: on load, focus, visibility and reconnect (FR-QUEUE-6). */
  readonly refresh: () => void;
  /** A ping of this room: it asks only when it tells of a newer queue (FR-QUEUE-4). */
  readonly ping: (ping: Ping) => void;
  /** The queue the server answered an action of this page with. */
  readonly accept: (queue: Queue) => void;
  /** Hands nothing over from now on. */
  readonly stop: () => void;
};

export type KeeperOptions = {
  readonly load: () => Promise<Queue>;
  readonly clock: Clock;
  readonly onQueue: (queue: Queue) => void;
  /** A fetch failed while there was nothing to show. A queue that was shown stays. */
  readonly onFailure: (failure: unknown) => void;
  /** NFR-PERF-1: from a ping's sentAt to the queue it told of being held, in milliseconds. */
  readonly onLatency: (ms: number, ping: Ping) => void;
};

/**
 * A ping older than this when it arrives was replayed after a reconnect, not delivered live
 * (notes/phase-0.md): it still asks for the queue, but its latency would say nothing.
 */
const LIVE_WITHIN_MS = 10_000;

export function keepQueue(options: KeeperOptions): Keeper {
  const { clock } = options;
  let held: Queue | null = null;
  let isStopped = false;
  // Live pings whose queue is not held yet, for their latency.
  let pending: readonly Ping[] = [];

  const take = (queue: Queue): void => {
    // An answer that lands late must never replace a newer queue.
    if (isStopped || (held && isOlderQueue(queue, held))) return;
    held = queue;
    const arrived = pending.filter((ping) => !isNewerPing(ping, queue));
    pending = pending.filter((ping) => isNewerPing(ping, queue));
    options.onQueue(queue);
    const at = clock.serverNow();
    arrived.forEach((ping) =>
      options.onLatency(Math.max(0, Math.round(at - ping.sentAt)), ping),
    );
  };

  const fetchOnce = async (): Promise<void> => {
    if (isStopped) return;
    const sentAt = clock.now();
    let queue: Queue;
    try {
      queue = await options.load();
    } catch (failure) {
      if (isStopped) return;
      pending = []; // the fetch those pings led to is gone
      if (!held) options.onFailure(failure);
      return;
    }
    if (isStopped) return;
    clock.sync(queue.serverTime, sentAt, clock.now());
    take(queue);
  };

  const refresh = oneAtATime(fetchOnce);

  return {
    refresh,
    ping: (ping) => {
      if (isStopped || !isNewerPing(ping, held)) return;
      // With nothing held the clock is not set yet, and the ping may be a replay of the past.
      const isLive = clock.serverNow() - ping.sentAt <= LIVE_WITHIN_MS;
      if (held && isLive) pending = [...pending, ping];
      refresh();
    },
    accept: take,
    stop: () => {
      isStopped = true;
      pending = [];
    },
  };
}
