import { describe, expect, it } from "vitest";

import { BOT_ID } from "../nexconn/cards";
import { NOW, liveSession, queues } from "./fixtures";
import { isNewerPing, isOlderQueue, readPing } from "./ping";
import type { PingMessage } from "./ping";
import type { Queue } from "./types";

/** The Nexconn group of the room that is open. */
const ROOM = "ohg7k2m9x4";

const message = (over: Partial<PingMessage> = {}): PingMessage => ({
  messageType: "OH:Queue",
  senderUserId: BOT_ID,
  channelIdentifier: { channelId: ROOM },
  content: { sessionId: "12", version: 10, sentAt: 1_763_120_000_000 },
  ...over,
});

describe("readPing", () => {
  it("FR-QUEUE-4: reads the bot's ping of this room", () => {
    expect(readPing(message(), ROOM)).toEqual({
      sessionId: "12",
      version: 10,
      sentAt: 1_763_120_000_000,
    });
  });

  it("FR-QUEUE-7, AT-15: ignores a ping the bot did not send", () => {
    expect(readPing(message({ senderUserId: "gh42" }), ROOM)).toBeNull();
    expect(readPing(message({ senderUserId: "" }), ROOM)).toBeNull();
  });

  it("ignores a ping of another room the person is in", () => {
    const elsewhere = message({ channelIdentifier: { channelId: "ohother" } });
    expect(readPing(elsewhere, ROOM)).toBeNull();
  });

  it("ignores a message that names no room", () => {
    expect(readPing(message({ channelIdentifier: null }), ROOM)).toBeNull();
    expect(readPing(message({ channelIdentifier: {} }), ROOM)).toBeNull();
    expect(
      readPing(message({ channelIdentifier: undefined }), ROOM),
    ).toBeNull();
  });

  it("ignores every other kind of message", () => {
    for (const messageType of ["OH:Hand", "OH:FAQ", "RC:TxtMsg", ""]) {
      expect(readPing(message({ messageType }), ROOM)).toBeNull();
    }
  });

  it("ignores broken content", () => {
    const good = { sessionId: "12", version: 10, sentAt: 1_763_120_000_000 };
    const broken: readonly unknown[] = [
      null,
      undefined,
      "text",
      42,
      [],
      {},
      { ...good, sessionId: 12 },
      { ...good, sessionId: "" },
      { ...good, sessionId: "twelve" },
      { ...good, sessionId: "-12" },
      { ...good, sessionId: "1.5" },
      { ...good, version: "10" },
      { ...good, version: 10.5 },
      { ...good, version: -1 },
      { ...good, version: Number.NaN },
      { ...good, sentAt: "now" },
      { ...good, sentAt: Number.POSITIVE_INFINITY },
      { ...good, sentAt: Number.NaN },
      { version: 10, sentAt: 1 },
      { sessionId: "12", sentAt: 1 },
      { sessionId: "12", version: 10 },
    ];
    for (const content of broken) {
      expect(readPing(message({ content }), ROOM)).toBeNull();
    }
  });
});

describe("isNewerPing", () => {
  /** Session 12 at version 9. */
  const held = queues.liveWatching;
  const ping = (sessionId: string, version: number) => ({
    sessionId,
    version,
    sentAt: 0,
  });

  it("FR-QUEUE-4: the same session at a greater version is newer", () => {
    expect(isNewerPing(ping("12", 10), held)).toBe(true);
    expect(isNewerPing(ping("12", 40), held)).toBe(true);
  });

  it("FR-QUEUE-4: the version that is held, or an older one, is not", () => {
    expect(isNewerPing(ping("12", 9), held)).toBe(false);
    expect(isNewerPing(ping("12", 8), held)).toBe(false);
    expect(isNewerPing(ping("12", 0), held)).toBe(false);
  });

  it("a greater session id is newer whatever its version, and ids compare as numbers", () => {
    expect(isNewerPing(ping("13", 1), held)).toBe(true);
    expect(isNewerPing(ping("100", 1), held)).toBe(true); // "100" < "12" as text
    expect(isNewerPing(ping("11", 99), held)).toBe(false);
    expect(isNewerPing(ping("9", 99), held)).toBe(false); // "9" > "12" as text
  });

  it("with nothing held, any ping of this room is newer", () => {
    expect(isNewerPing(ping("1", 1), null)).toBe(true);
    expect(isNewerPing(ping("1", 1), queues.noSession)).toBe(true);
  });

  it("a session that ended is still the one that is held", () => {
    expect(isNewerPing(ping("12", 31), queues.ended)).toBe(false);
    expect(isNewerPing(ping("13", 1), queues.ended)).toBe(true);
  });
});

describe("isOlderQueue", () => {
  const at = (version: number, serverTime: string = NOW): Queue => ({
    ...queues.liveWatching,
    session: { ...liveSession, version },
    serverTime,
  });
  const ofSession = (id: string, version: number): Queue => ({
    ...queues.liveWatching,
    session: { ...liveSession, id, version },
  });

  it("an answer of an earlier version is older than what is held", () => {
    expect(isOlderQueue(at(8), at(9))).toBe(true);
    expect(isOlderQueue(at(10), at(9))).toBe(false);
  });

  it("an answer of an earlier session is older, and ids compare as numbers", () => {
    expect(isOlderQueue(ofSession("9", 99), ofSession("12", 1))).toBe(true);
    expect(isOlderQueue(ofSession("100", 1), ofSession("12", 99))).toBe(false);
  });

  it("no session is older than any session", () => {
    expect(isOlderQueue(queues.noSession, at(1))).toBe(true);
    expect(isOlderQueue(at(1), queues.noSession)).toBe(false);
  });

  it("at the same version the server's clock decides: the next session's time changes no version", () => {
    const earlier = at(9, "2026-11-14T11:50:00.000Z");
    const later = at(9, "2026-11-14T11:50:02.500Z");
    expect(isOlderQueue(earlier, later)).toBe(true);
    expect(isOlderQueue(later, earlier)).toBe(false);
  });

  it("the queue that is held is not older than itself", () => {
    expect(isOlderQueue(at(9), at(9))).toBe(false);
    expect(isOlderQueue(queues.noSession, queues.noSession)).toBe(false);
  });
});
