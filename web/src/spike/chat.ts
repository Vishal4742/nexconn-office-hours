// P0 spike: Nexconn Chat SDK + Chat UI as one module-level singleton. React Strict Mode and hot
// reload run effects twice; the engine and Chat UI may only be initialized once per page.
import {
  AreaCode,
  BaseChannel,
  ChannelType,
  ConnectionStatus,
  ConnectionStatusHandler,
  GroupChannelIdentifier,
  LogLevel,
  MessageHandler,
  NCEngine,
} from "@nexconn/chat";
import type { Message } from "@nexconn/chat";
import {
  BubbleLayout,
  InputMenumID,
  NCChatUIApplication,
} from "@nexconn/chatui";
import type { ServiceHooks } from "@nexconn/chatui";

import { OhFaqCard, OhHandCard, OhSessionCard } from "../nexconn/cardElements";
import { BOT_ID, isFromBot } from "../nexconn/cards";

export const SPIKE_GROUP = "ohspike1";
const NAMES: Readonly<Record<string, string>> = {
  [BOT_ID]: "Office Hours",
  ohspikea: "Spike A",
  ohspikeb: "Spike B",
};
const AREAS: Readonly<Record<string, AreaCode>> = {
  bj: AreaCode.BJ,
  sg: AreaCode.SG,
  na: AreaCode.NA,
  sa: AreaCode.SA,
  om: AreaCode.OM,
};
/** Chat UI's "channel list not ready yet" code: retry openChannel shortly after. */
const CHANNEL_LIST_NOT_READY = 39005;

export type SpikeState = {
  readonly phase: "starting" | "ready" | "failed";
  readonly error: string;
  readonly userId: string;
  readonly connection: string;
  readonly ohMessages: number; // OH:Session, OH:Hand and OH:FAQ received live from the bot
  readonly pings: number; // OH:Queue pings from the bot
  readonly lastPing: string;
  readonly ignored: number; // OH:* messages from anyone but the bot
  readonly unread: string; // the room's unread count and @ badge, as the SDK reports them
};

let state: SpikeState = {
  phase: "starting",
  error: "",
  userId: "",
  connection: "not connected",
  ohMessages: 0,
  pings: 0,
  lastPing: "none yet",
  ignored: 0,
  unread: "not checked",
};
const listeners = new Set<() => void>();

function update(patch: Partial<SpikeState>): void {
  state = { ...state, ...patch };
  listeners.forEach((listener) => listener());
}

export function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getState(): SpikeState {
  return state;
}

// Chat UI will not render without these. Every profile needs a non-empty name.
const hooks: ServiceHooks = {
  reqUserProfiles: async (userIds) =>
    userIds.map((userId) => ({ userId, name: NAMES[userId] ?? userId })),
  reqGroupProfiles: async (groupIds) =>
    groupIds.map((groupId) => ({
      groupId,
      name: "Office Hours spike",
      memberCount: Object.keys(NAMES).length,
    })),
  reqSystemProfiles: async () => [],
  reqGroupMembers: async () => Object.keys(NAMES).map((userId) => ({ userId })),
};

/** FR-QUEUE-7: act on OH:* messages only when the bot sent them. */
function onMessage(message: Message): void {
  if (!message.messageType.startsWith("OH:")) return;
  if (!isFromBot(message)) {
    update({ ignored: state.ignored + 1 });
    return;
  }
  if (message.messageType !== "OH:Queue") {
    update({ ohMessages: state.ohMessages + 1 });
    return;
  }
  const content: unknown = message.content;
  const sentAt =
    typeof content === "object" &&
    content !== null &&
    "sentAt" in content &&
    typeof content.sentAt === "number"
      ? content.sentAt
      : 0;
  const delay = sentAt
    ? `${Date.now() - sentAt} ms after the server sent it`
    : "no sentAt in the ping";
  update({
    pings: state.pings + 1,
    lastPing: `${delay} (status message: ${message.isStatusMessage})`,
  });
}

let unreadTimer: number | undefined;
const RETRY_MS = 250;
const pause = () => new Promise((resolve) => setTimeout(resolve, RETRY_MS));

/**
 * A-12: does the room count as unread, and as "mentions me"? Only meaningful with ?open=0.
 * Answers false while the SDK's channel list is still syncing (code 35011 right after connect).
 */
async function refreshUnread(): Promise<boolean> {
  const types = [ChannelType.GROUP];
  const [unread, mentions] = await Promise.all([
    BaseChannel.getUnreadChannels(types),
    BaseChannel.createUnreadMentionMeChannelsQuery({
      channelTypes: types,
    }).loadNextPage(),
  ]);
  if (!unread.isOk || !mentions.isOk) {
    update({ unread: `failed (codes ${unread.code}, ${mentions.code})` });
    return false;
  }
  const room = unread.data?.find((c) => c.channelId === SPIKE_GROUP);
  const isMentioned =
    mentions.data?.data.some((c) => c.channelId === SPIKE_GROUP) ?? false;
  update({
    unread: `${room?.unreadCount ?? 0} unread, @ badge: ${isMentioned ? "yes" : "no"}`,
  });
  return true;
}

async function json(path: string): Promise<Record<string, string>> {
  const res = await fetch(path);
  // A proxy or platform error page is HTML: report the status, not a JSON parse error.
  const body: unknown = await res.json().catch(() => null);
  if (
    !res.ok ||
    typeof body !== "object" ||
    body === null ||
    !("data" in body)
  ) {
    throw new Error(`${path} answered ${res.status} without the expected JSON`);
  }
  return body.data as Record<string, string>;
}

async function boot(
  user: string,
  isOpen: boolean,
): Promise<NCChatUIApplication> {
  const [config, auth] = await Promise.all([
    json("/api/config"),
    json(`/api/spike/token?user=${user}`),
  ]);

  NCEngine.initialize({
    appKey: config.appKey,
    areaCode: AREAS[config.area] ?? AreaCode.SG,
    logLevel: LogLevel.WARN,
  });
  const app = NCChatUIApplication.initialize({ hooks, language: "en_US" });
  if (!app) throw new Error("Chat UI did not initialize");

  // Everything below must happen before ready().
  app.registerCustomMessages([
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
      messageType: "OH:Queue",
      isPersisted: false,
      isCounted: false,
      isStatusMessage: true,
    },
  ]);
  app.setMessageBubbleConfig({
    ...app.cloneMessageBubbleConfig(),
    redius: 12, // the SDK's spelling
    layout: BubbleLayout.LEFT_JUSTIFYING,
    backgroundColorForMyself: 0xe6edff,
    backgroundColorForOthers: 0xf3f4f6,
    textColorForMyself: 0x14151a,
    textColorForOthers: 0x14151a,
  });
  const menu = app.cloneInputMenu();
  const keep: readonly string[] = [
    InputMenumID.EMOJI,
    InputMenumID.IMAGES,
    InputMenumID.FILES,
  ];
  app.setInputMenu({
    ...menu,
    items: menu.items.filter((item) => keep.includes(item.id)),
  });
  app.ready();

  // Handlers go in before connect, so nothing is missed.
  NCEngine.addConnectionStatusHandler(
    "oh-spike",
    new ConnectionStatusHandler({
      onConnectionStatusChanged: ({ status }) =>
        update({ connection: ConnectionStatus[status] ?? String(status) }),
    }),
  );
  NCEngine.addMessageHandler(
    "oh-spike",
    new MessageHandler({
      onMessageReceived: ({ messages }) => {
        messages.forEach(onMessage);
        if (isOpen) return; // an open room marks everything as read
        // The SDK updates its channel list after this handler returns. A burst becomes one check.
        window.clearTimeout(unreadTimer);
        unreadTimer = window.setTimeout(() => {
          refreshUnread().catch(() => update({ unread: "the check threw" }));
        }, 1000);
      },
    }),
  );

  const connected = await NCEngine.connect({ token: auth.token });
  if (!connected.isOk)
    throw new Error(`Nexconn refused the connection (code ${connected.code})`);
  update({ userId: NCEngine.getCurrentUserId(), connection: "CONNECTED" });

  // Chat UI copies sender names and avatars out of its profile cache the moment a room opens,
  // before reqUserProfiles has answered, and never refreshes that copy. Fill the cache first.
  Object.entries(NAMES).forEach(([userId, name]) =>
    app.updateUserProfile({ userId, name }),
  );

  if (!isOpen) {
    // ?open=0: stay out of the room, so nothing is marked as read.
    for (let attempt = 0; attempt < 20 && !(await refreshUnread()); attempt++)
      await pause();
    update({ phase: "ready" });
    return app;
  }

  for (let attempt = 0; attempt < 20; attempt++) {
    const opened = await app.openChannel(
      new GroupChannelIdentifier(SPIKE_GROUP),
      true,
    );
    if (opened.isOk) {
      update({ phase: "ready" });
      return app;
    }
    if (opened.code !== CHANNEL_LIST_NOT_READY)
      throw new Error(`could not open the room (code ${opened.code})`);
    await pause();
  }
  throw new Error("the channel list never became ready");
}

// ponytail: one promise per page, and the first caller's arguments win. Ceiling: a failed start
// needs a page reload (the engine can't initialize twice), a hot edit of this file would start it
// twice, and a second room can't be opened. The real room (P1) splits "initialize once" from
// "open room(id)" and lets the connect step be retried.
let started: Promise<NCChatUIApplication> | null = null;

/** Starts the SDK once per page, whoever asks and however often. */
export function startChat(
  user: string,
  isOpen: boolean,
): Promise<NCChatUIApplication> {
  started ??= boot(user, isOpen).catch((error: unknown) => {
    update({
      phase: "failed",
      error: error instanceof Error ? error.message : String(error),
    });
    throw error;
  });
  return started;
}
