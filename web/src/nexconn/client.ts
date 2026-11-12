// The one Nexconn connection of a page (FR-AUTH-3). Every signed-in page connects, not only the
// room: Nexconn counts active users by client connections.
//
// Two steps on purpose. Starting the SDK and Chat UI can happen once per page and is never
// repeated. Connecting can fail (network, token) and can be tried again without a reload.
// This module pulls in the whole SDK, so it is only ever loaded through nexconn/lazy.ts.
import {
  AreaCode,
  ConnectionStatus,
  ConnectionStatusHandler,
  LogLevel,
  MessageHandler,
  NCEngine,
} from "@nexconn/chat";
import type { Message } from "@nexconn/chat";
import {
  BubbleLayout,
  InputMenumID,
  MessageMenuID,
  NCChatUIApplication,
} from "@nexconn/chatui";
import type { ServiceHooks } from "@nexconn/chatui";

import { api } from "../api";
import { OhFaqCard, OhHandCard, OhSessionCard } from "./cardElements";
import { connectWithFreshToken } from "./connect";
import { showNamesInReplies } from "./replyNames";
import { groupProfiles, membersOf, userProfiles } from "./roster";

export type Connection =
  | { readonly status: "idle" }
  | { readonly status: "connecting" }
  | { readonly status: "connected" }
  | { readonly status: "reconnecting" }
  | { readonly status: "failed"; readonly message: string };

const AREAS: Readonly<Record<string, AreaCode>> = {
  bj: AreaCode.BJ,
  sg: AreaCode.SG,
  na: AreaCode.NA,
  sa: AreaCode.SA,
  om: AreaCode.OM,
};
const HANDLER_ID = "office-hours";

let state: Connection = { status: "idle" };
const listeners = new Set<() => void>();
const messageListeners = new Set<(messages: readonly Message[]) => void>();

function set(next: Connection): void {
  state = next;
  listeners.forEach((listener) => listener());
}

export function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getConnection(): Connection {
  return state;
}

/** Calls back with every batch of messages the SDK delivers, replayed ones included. */
export function onMessages(
  listener: (messages: readonly Message[]) => void,
): () => void {
  messageListeners.add(listener);
  return () => {
    messageListeners.delete(listener);
  };
}

// Chat UI will not render without these. They answer from memory (nexconn/roster.ts): Chat UI
// awaits the members hook every time a room is selected and copies names at that moment.
const hooks: ServiceHooks = {
  reqUserProfiles: async (userIds) => userProfiles(userIds),
  reqGroupProfiles: async (groupIds) => groupProfiles(groupIds),
  reqSystemProfiles: async () => [],
  reqGroupMembers: async (groupId) => membersOf(groupId),
};

let app: NCChatUIApplication | null = null;

/** Chat UI, once the SDK has started. The room uses it to open a channel. */
export function getApp(): NCChatUIApplication | null {
  return app;
}

/**
 * Starts the engine and Chat UI. Everything Chat UI must know before ready() is given here: the
 * card types and their elements, the bubble look and the input menu can't be added later, and
 * Chat UI only learns that the engine connected through a handler it registers now.
 */
function startOnce(config: {
  readonly appKey: string;
  readonly area: string;
}): NCChatUIApplication {
  if (app) return app;
  NCEngine.initialize({
    appKey: config.appKey,
    areaCode: AREAS[config.area] ?? AreaCode.SG,
    logLevel: LogLevel.WARN,
  });
  const created = NCChatUIApplication.initialize({ hooks, language: "en_US" });
  if (!created) throw new Error("The chat could not start in this browser.");

  created.registerCustomMessages([
    {
      messageType: "OH:Session",
      isPersisted: true,
      isCounted: true,
      digest: () => "Office hours",
      component: { tagName: "oh-session-card", elementClass: OhSessionCard },
    },
    {
      messageType: "OH:Hand",
      isPersisted: true,
      isCounted: true,
      digest: () => "A hand was raised",
      component: { tagName: "oh-hand-card", elementClass: OhHandCard },
    },
    {
      messageType: "OH:FAQ",
      isPersisted: true,
      isCounted: true,
      digest: () => "Added to FAQ",
      component: { tagName: "oh-faq-card", elementClass: OhFaqCard },
    },
    {
      messageType: "OH:Queue", // the queue's version ping: never shown, never counted
      isPersisted: false,
      isCounted: false,
      isStatusMessage: true,
    },
  ]);
  created.setMessageBubbleConfig({
    ...created.cloneMessageBubbleConfig(),
    redius: 12, // the SDK's spelling
    layout: BubbleLayout.LEFT_JUSTIFYING,
    backgroundColorForMyself: 0xe6edff,
    backgroundColorForOthers: 0xf1f0ec,
    textColorForMyself: 0x14151a,
    textColorForOthers: 0x14151a,
  });
  const menu = created.cloneInputMenu();
  const keep: readonly string[] = [
    InputMenumID.EMOJI,
    InputMenumID.IMAGES,
    InputMenumID.FILES,
  ];
  created.setInputMenu({
    ...menu,
    items: menu.items.filter((item) => keep.includes(item.id)),
  });
  // A room has no other channel to forward to, and nothing to do with several messages at once.
  const onAMessage: readonly string[] = [
    MessageMenuID.REPLY,
    MessageMenuID.COPY,
    MessageMenuID.DELETE_FOR_ME,
    MessageMenuID.DELETE_FOR_ALL,
  ];
  created.setMessageMenu(
    created.cloneMessageMenu().filter((item) => onAMessage.includes(item.id)),
  );
  created.ready();
  showNamesInReplies();

  NCEngine.addConnectionStatusHandler(
    HANDLER_ID,
    new ConnectionStatusHandler({
      onConnectionStatusChanged: ({ status }) => {
        // Only a connection that was up can be "reconnecting"; the first attempt reports itself.
        if (status === ConnectionStatus.CONNECTED) set({ status: "connected" });
        else if (
          state.status === "connected" ||
          state.status === "reconnecting"
        )
          set({ status: "reconnecting" });
      },
    }),
  );
  NCEngine.addMessageHandler(
    HANDLER_ID,
    new MessageHandler({
      onMessageReceived: ({ messages }) =>
        messageListeners.forEach((listener) => listener(messages)),
    }),
  );
  app = created;
  return created;
}

let connecting: Promise<void> | null = null;

/**
 * Connects this page to Nexconn as the signed-in user. Safe to call from every page and as
 * often as wanted; after a failure the next call tries again.
 */
export function ensureConnected(): Promise<void> {
  connecting ??= (async () => {
    set({ status: "connecting" });
    try {
      startOnce(await api.get<{ appKey: string; area: string }>("/api/config"));
      const outcome = await connectWithFreshToken({
        fetchToken: async () =>
          (await api.post<{ token: string }>("/api/nexconn/token")).token,
        connect: async (token) => {
          const result = await NCEngine.connect({ token });
          return result.isOk ? { ok: true } : { ok: false, code: result.code };
        },
      });
      if (!outcome.ok) {
        throw new Error(
          `The chat service refused the connection (code ${outcome.code}).`,
        );
      }
      set({ status: "connected" });
    } catch (error) {
      connecting = null; // the next call may try again
      set({
        status: "failed",
        message:
          error instanceof Error
            ? error.message
            : "The chat could not connect.",
      });
    }
  })();
  return connecting;
}

/** FR-AUTH-4: signing out leaves Nexconn too. */
export async function disconnect(): Promise<void> {
  connecting = null;
  if (app) await NCEngine.disconnect();
  set({ status: "idle" });
}

// The engine and Chat UI can start once per page. In development an edit to this file (or to
// anything it imports) would start them a second time, so the page reloads.
if (import.meta.hot) {
  import.meta.hot.accept(() => window.location.reload());
}
