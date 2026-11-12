// Opening one room in Chat UI. This file imports the SDK, so it is only ever loaded through
// nexconn/lazy.ts, like client.ts.
import { GroupChannelIdentifier } from "@nexconn/chat";

import { ensureConnected, getApp, getConnection, onMessages } from "./client";
import { inTurn, openWhenReady } from "./connect";
import { membersOf, rememberRoom, strangers, userProfiles } from "./roster";
import type { Person } from "./roster";

export type RoomToOpen = {
  readonly groupId: string;
  readonly name: string;
  /** Asks our server who is in the room: before it opens, and again when a stranger writes. */
  readonly loadMembers: () => Promise<readonly Person[]>;
};

const RETRY_MS = 250;
const pause = () =>
  new Promise<void>((resolve) => setTimeout(resolve, RETRY_MS));

/**
 * Opens the room's channel and puts Chat UI's message list and input into `host`. Resolves to a
 * function that takes them out again. If `signal` is aborted on the way (the page was left),
 * nothing is put in.
 */
export async function openRoom(
  host: HTMLElement,
  room: RoomToOpen,
  signal: AbortSignal,
): Promise<() => void> {
  await ensureConnected();
  const app = getApp();
  const connection = getConnection();
  if (!app || connection.status === "failed") {
    throw new Error(
      connection.status === "failed"
        ? connection.message
        : "The chat could not start.",
    );
  }
  if (signal.aborted) return () => {};

  // Chat UI copies names and avatars the moment a room is selected and never refreshes that
  // copy on its own, so it must know everyone before the channel opens (docs/ARCHITECTURE.md §4).
  let asks = 0;
  const tellChatUi = async () => {
    const ask = ++asks;
    const members = await room.loadMembers();
    // An older answer that arrives after a newer one would take the newest people out again.
    if (ask !== asks) return;
    rememberRoom(room.groupId, room.name, members);
    const ids = membersOf(room.groupId).map((member) => member.userId);
    userProfiles(ids).forEach((profile) => app.updateUserProfile(profile));
  };
  await tellChatUi();
  if (signal.aborted) return () => {};

  // Rooms are opened one at a time, and a page that was left stops trying: otherwise the room
  // of the page before could end up under this one.
  const hasLeft = () => signal.aborted;
  const opened = await inTurn(async () =>
    hasLeft()
      ? null
      : openWhenReady(
          () => app.openChannel(new GroupChannelIdentifier(room.groupId), true),
          pause,
          20,
          hasLeft,
        ),
  );
  if (!opened || hasLeft()) return () => {};
  if (!opened.isOk) {
    throw new Error(`The room could not be opened (code ${opened.code}).`);
  }

  const toast = document.createElement("nc-toast-provider");
  const list: HTMLElement & { requestUpdate?: () => void } =
    document.createElement("nc-message-list-provider");
  list.className = "nc-chatui-msg-list";
  const input: HTMLElement & { model?: unknown } =
    document.createElement("nc-input-provider");
  // An input keeps writing to the room it was made for, so it gets the room that is open now.
  input.model = app.getOpenedChannel();
  host.replaceChildren(toast, list, input);

  // Someone who joined after the room was opened shows as a raw id until Chat UI is told who
  // they are. Each stranger is asked about once, so an id our server doesn't know can't loop.
  const asked = new Set<string>();
  const stopListening = onMessages((messages) => {
    const unknown = strangers(
      room.groupId,
      messages.map((message) => ({
        groupId: message.channelIdentifier.channelId,
        senderUserId: message.senderUserId,
      })),
    ).filter((userId) => !asked.has(userId));
    if (unknown.length === 0) return;
    unknown.forEach((userId) => asked.add(userId));
    void tellChatUi().then(
      () => {
        app.updateGroupMembers(room.groupId, [...membersOf(room.groupId)]);
        setTimeout(() => list.requestUpdate?.(), 0);
      },
      () => {
        // The list could not be loaded. The names stay as they are until the next stranger.
        unknown.forEach((userId) => asked.delete(userId));
      },
    );
  });

  return () => {
    stopListening();
    host.replaceChildren();
  };
}
