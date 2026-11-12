// One clock for the room (plan decision 12): the server's. The page's own clock can be minutes
// off, so every answer of the server carries its time, and the page keeps the difference.

/**
 * A time the server wrote, in milliseconds. Go writes up to nine digits after the second, and
 * Date.parse is only sure of three, so the rest are dropped first. Not a number when it is not
 * a time.
 */
export function toMs(time: string): number {
  return Date.parse(
    time.replace(
      /\.(\d+)/,
      (_, digits: string) => `.${digits.slice(0, 3).padEnd(3, "0")}`,
    ),
  );
}

/**
 * How far the server's clock is ahead of the page's, from one fetch: the server's time against
 * the middle of the fetch. Null when the answer holds no time that can be read.
 */
export function clockOffset(
  serverTime: string,
  sentAt: number,
  answeredAt: number,
): number | null {
  const server = toMs(serverTime);
  return Number.isNaN(server) ? null : server - (sentAt + answeredAt) / 2;
}

export type Clock = {
  /** The page's clock. */
  readonly now: () => number;
  /** The server's clock: the page's plus the difference kept from the last fetch. */
  readonly serverNow: () => number;
  /** Keeps the difference one fetch measured. */
  readonly sync: (
    serverTime: string,
    sentAt: number,
    answeredAt: number,
  ) => void;
};

/**
 * A clock that counts on `pageNow` until a fetch was answered, then on the server's time.
 * `ponytail:` the difference of the latest fetch wins, off by at most half its round trip; keep
 * the one of the shortest round trip if minutes ever need to be exact to the second.
 */
export function makeClock(pageNow: () => number): Clock {
  let offset = 0;
  return {
    now: pageNow,
    serverNow: () => pageNow() + offset,
    sync: (serverTime, sentAt, answeredAt) => {
      offset = clockOffset(serverTime, sentAt, answeredAt) ?? offset;
    },
  };
}

/** How many latencies are kept for NFR-PERF-1. */
const KEPT = 50;

/** The list with one more latency, the oldest dropped beyond the last 50. */
export function remember(
  latencies: readonly number[],
  ms: number,
): readonly number[] {
  return [...latencies, ms].slice(-KEPT);
}
