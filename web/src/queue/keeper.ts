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
  /**
   * Called every few seconds while the page is visible: asks again when the room has been quiet
   * for too long (QUIET_LIVE_MS, QUIET_IDLE_MS). A connection can die without a reconnect event,
   * and its pings stop with it (FR-QUEUE-6).
   */
  readonly checkSilence: () => void;
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

/**
 * How long a room may stay quiet before the page asks for its queue anyway. Telegram's clients
 * fetch the difference after 15 minutes without updates (core.telegram.org/api/updates); a live
 * queue moves by the minute, so a live session gets one minute, and a room without one five,
 * which is how soon it notices a session that started while its pings were lost.
 */
export const QUIET_LIVE_MS = 60_000;
export const QUIET_IDLE_MS = 5 * 60_000;

export function keepQueue(options: KeeperOptions): Keeper {
  const { clock } = options;
  let held: Queue | null = null;
  let isStopped = false;
  // Live pings whose queue is not held yet, for their latency.
  let pending: readonly Ping[] = [];
  // Since when nothing came from the server about this room (no answer, no ping) and nothing was asked.
  let quietSince = clock.now();
  const hear = (): void => {
    quietSince = clock.now();
  };

  const take = (queue: Queue): void => {
    if (isStopped) return;
    hear();
    // An answer that lands late must never replace a newer queue.
    if (held && isOlderQueue(queue, held)) return;
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
      if (isStopped) return;
      hear(); // even a ping of the queue it holds says the line to the room is alive
      if (!isNewerPing(ping, held)) return;
      // With nothing held the clock is not set yet, and the ping may be a replay of the past.
      const isLive = clock.serverNow() - ping.sentAt <= LIVE_WITHIN_MS;
      if (held && isLive) pending = [...pending, ping];
      refresh();
    },
    accept: take,
    checkSilence: () => {
      if (isStopped || !held) return;
      const limit = held.session?.live ? QUIET_LIVE_MS : QUIET_IDLE_MS;
      if (clock.now() - quietSince < limit) return;
      hear(); // the ask counts too: a server that does not answer is asked once per quiet spell
      refresh();
    },
    stop: () => {
      isStopped = true;
      pending = [];
    },
  };
}
