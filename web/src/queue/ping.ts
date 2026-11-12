// The queue's version ping, OH:Queue {sessionId, version, sentAt} (server/notify.go). It is a
// hint, never state: it only makes the page ask the server for the queue (plan decision 4), and
// the server's answers are kept in order.
import { isFromBot } from "../nexconn/cards";
import { toMs } from "./clock";
import type { Queue, QueueSession } from "./types";

export type Ping = {
  readonly sessionId: string;
  readonly version: number;
  /** When the server sent it, in Unix milliseconds of the server's clock. */
  readonly sentAt: number;
};

/** What is read of a Nexconn message: the field names of @nexconn/chat's Message (index.d.ts). */
export type PingMessage = {
  readonly messageType: string;
  readonly senderUserId: string;
  readonly channelIdentifier?: { readonly channelId?: string } | null;
  readonly content: unknown;
};

const QUEUE_PING = "OH:Queue";
const SESSION_ID = /^[1-9]\d*$/;

/**
 * The ping in a message, or null. Only the bot's pings count (FR-QUEUE-7, AT-15), and only those
 * of this room: session ids are global, so a ping of another room the person is in would look
 * newer than this room's queue.
 */
export function readPing(message: PingMessage, groupId: string): Ping | null {
  if (message.messageType !== QUEUE_PING || !isFromBot(message)) return null;
  if (message.channelIdentifier?.channelId !== groupId) return null;
  const { content } = message;
  if (typeof content !== "object" || content === null) return null;
  const { sessionId, version, sentAt } = content as Record<string, unknown>;
  const isPing =
    typeof sessionId === "string" &&
    SESSION_ID.test(sessionId) &&
    typeof version === "number" &&
    Number.isSafeInteger(version) &&
    version >= 0 &&
    typeof sentAt === "number" &&
    Number.isFinite(sentAt);
  return isPing ? { sessionId, version, sentAt } : null;
}

/** Ids are whole numbers written as text: the longer one is greater, so "100" > "12". */
function compareIds(a: string, b: string): number {
  return a.length - b.length || (a < b ? -1 : a > b ? 1 : 0);
}

/** Orders sessions: no session first, then by id (ids only grow), then by version. */
function compareSessions(
  a: QueueSession | null,
  b: QueueSession | null,
): number {
  if (!a || !b) return (a ? 1 : 0) - (b ? 1 : 0);
  return compareIds(a.id, b.id) || a.version - b.version;
}

/**
 * FR-QUEUE-4: whether a ping tells of a queue newer than the one held: a greater session id, or
 * the same session at a greater version. With nothing held, any ping of the room does.
 */
export function isNewerPing(ping: Ping, held: Queue | null): boolean {
  const session = held?.session;
  if (!session) return true;
  const order =
    compareIds(ping.sessionId, session.id) || ping.version - session.version;
  return order > 0;
}

/**
 * Whether an answer of the server is older than the queue held, so that it must not replace it.
 * At the same version the server's clock decides: the next session's time changes no version.
 */
export function isOlderQueue(candidate: Queue, held: Queue): boolean {
  const order = compareSessions(candidate.session, held.session);
  if (order !== 0) return order < 0;
  return toMs(candidate.serverTime) < toMs(held.serverTime);
}
