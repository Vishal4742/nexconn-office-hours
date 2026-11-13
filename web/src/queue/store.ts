// The queue of the room that is open, kept current for everything that shows it: the panel, the
// room's head, the your-turn banner and the hand cards in the chat. The exports listed in
// notes/phase-2-plan.md are a contract. The queue is loaded on watch, again when the window gets
// focus or becomes visible, when the Nexconn connection comes (back) up, when a newer ping of this
// room arrives, and when a visible room has been quiet too long (FR-QUEUE-4, FR-QUEUE-6); keeper.ts
// holds the rules.
import { useSyncExternalStore } from "react";

import { errorMessage } from "../api";
import { loadClient } from "../nexconn/lazy";
import type { RepoName } from "../projects";
import { loadQueue } from "./api";
import { makeClock, remember } from "./clock";
import { keepQueue } from "./keeper";
import type { Keeper } from "./keeper";
import { readPing } from "./ping";
import type { Ping } from "./ping";
import type { Hand, Queue } from "./types";

export type QueueState =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly queue: Queue }
  /** Only when there is nothing to show: a queue that was loaded stays on screen. */
  | { readonly status: "failed"; readonly message: string };

/** The room whose queue is watched. groupId is its Nexconn group: pings arrive in it. */
export type WatchedRoom = RepoName & {
  readonly groupId: string;
  /** Whether the viewer maintains the project: what a card may offer (the server still checks). */
  readonly isMaintainer: boolean;
};

type Watch = {
  readonly room: WatchedRoom;
  readonly keeper: Keeper;
  readonly stop: () => void;
};

let current: Watch | null = null;
let state: QueueState = { status: "loading" };
const listeners = new Set<() => void>();
const clock = makeClock(() => Date.now());
let latencies: readonly number[] = [];
/** How long to wait before downloading the chat's code again after it failed. */
const RETRY_MS = 5_000;
/** How often a visible page asks the keeper whether its room has gone quiet for too long. */
const QUIET_CHECK_MS = 15_000;

function set(next: QueueState): void {
  state = next;
  listeners.forEach((listener) => listener());
}

/** Calls back whenever the queue changes. For code outside React, such as the chat's cards. */
export function subscribeQueue(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getQueueState(): QueueState {
  return state;
}

/** The queue of the watched room. */
export function useQueue(): QueueState {
  return useSyncExternalStore(subscribeQueue, getQueueState);
}

/** Asks the server for the queue now. After a failure, the panel shows that it is trying. */
export function refreshQueue(): void {
  if (!current) return;
  if (state.status === "failed") set({ status: "loading" });
  current.keeper.refresh();
}

/** NFR-PERF-1: one line per ping, which AT-16 reads from the console. */
function logLatency(ms: number, ping: Ping): void {
  latencies = remember(latencies, ms);
  console.debug(
    `NFR-PERF-1 queue ping to store: ${ms} ms (session ${ping.sessionId}, version ${ping.version})`,
  );
}

/**
 * The triggers besides the first load: focus, visibility, the connection coming (back) up, the
 * room's pings, and a room that went quiet while the page is in view. The SDK is reached only
 * through loadClient(): a static import would put 1.4 MB into the first page.
 */
function listen(room: WatchedRoom, keeper: Keeper): () => void {
  const onFocus = () => keeper.refresh();
  const onVisible = () => {
    if (document.visibilityState === "visible") keeper.refresh();
  };
  window.addEventListener("focus", onFocus);
  document.addEventListener("visibilitychange", onVisible);
  // A hidden page asks again when it becomes visible; only one in view needs the quiet check.
  const quietCheck = setInterval(() => {
    if (document.visibilityState === "visible") keeper.checkSilence();
  }, QUIET_CHECK_MS);

  let isListening = true;
  let stopClient = () => {};
  let retry: ReturnType<typeof setTimeout> | undefined;
  const attach = (isLate: boolean) =>
    loadClient().then(
      (client) => {
        if (!isListening) return;
        const stopMessages = client.onMessages((messages) =>
          messages.forEach((message) => {
            const ping = readPing(message, room.groupId);
            if (ping) keeper.ping(ping);
          }),
        );
        let wasConnected = client.getConnection().status === "connected";
        const stopConnection = client.subscribe(() => {
          const isConnected = client.getConnection().status === "connected";
          if (isConnected && !wasConnected) keeper.refresh();
          wasConnected = isConnected;
        });
        stopClient = () => {
          stopMessages();
          stopConnection();
        };
        if (isLate) keeper.refresh(); // pings may have been missed meanwhile
      },
      () => {
        // The chat's code could not be downloaded; the connection banner says so. Without it no
        // ping arrives, and a page in front would never learn of a change: so ask again.
        if (isListening) retry = setTimeout(() => attach(true), RETRY_MS);
      },
    );
  void attach(false);

  return () => {
    isListening = false;
    clearTimeout(retry);
    clearInterval(quietCheck);
    window.removeEventListener("focus", onFocus);
    document.removeEventListener("visibilitychange", onVisible);
    stopClient();
  };
}

/**
 * Starts keeping one room's queue current, and returns the function that stops it. One room at a
 * time: the room page calls this in an effect, and a new watch ends the one before.
 */
export function watchQueue(room: WatchedRoom): () => void {
  current?.stop();
  const keeper = keepQueue({
    load: () => loadQueue(room),
    clock,
    onQueue: (queue) => set({ status: "ready", queue }),
    onFailure: (failure) =>
      set({ status: "failed", message: errorMessage(failure) }),
    onLatency: logLatency,
  });
  const stopListening = listen(room, keeper);
  const watch: Watch = {
    room,
    keeper,
    stop: () => {
      stopListening();
      keeper.stop();
    },
  };
  current = watch;
  set({ status: "loading" });
  keeper.refresh();
  return () => {
    if (current !== watch) return;
    watch.stop();
    current = null;
    set({ status: "loading" });
  };
}

/** The room being watched, or null. The chat's cards build their links from it. */
export function watchedRoom(): WatchedRoom | null {
  return current?.room ?? null;
}

/**
 * Takes the queue the server answered an action of this page with. An answer older than the
 * queue held is dropped: it must never replace a newer one.
 */
export function acceptQueue(queue: Queue): void {
  current?.keeper.accept(queue);
}

/**
 * Makes a call that answers with the queue, and gives the answer to acceptQueue unless the page
 * moved to another room meanwhile. Session ids are global, so another room's queue could look
 * newer than this room's and keep its real queue out.
 */
export async function actOnQueue(call: () => Promise<Queue>): Promise<void> {
  const groupId = current?.room.groupId;
  const queue = await call();
  if (current?.room.groupId === groupId) acceptQueue(queue);
}

/** A hand of the watched room's session by id, or null when the queue does not hold it. */
export function findHand(handId: string): Hand | null {
  if (state.status !== "ready") return null;
  const { hands, done } = state.queue;
  return (
    hands.find((hand) => hand.id === handId) ??
    done.find((hand) => hand.id === handId) ??
    null
  );
}

/** The server's clock, as well as this page knows it (plan decision 12): count minutes with it. */
export function serverNow(): number {
  return clock.serverNow();
}

/** NFR-PERF-1: the last 50 latencies from a ping to its queue in the store, in milliseconds. */
export function queueLatencies(): readonly number[] {
  return latencies;
}
