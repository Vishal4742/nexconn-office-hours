// The queue of the room that is open, kept current for everything that shows it: the panel, the
// room's head, the your-turn banner and the hand cards in the chat. The exports are a contract
// (notes/phase-2-plan.md); how the queue is kept current is the queue panel's part of the build
// (worker 5). For now it is loaded once, and again after each action of this page.
import { useSyncExternalStore } from "react";

import { errorMessage } from "../api";
import type { RepoName } from "../projects";
import { loadQueue } from "./api";
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

let watched: WatchedRoom | null = null;
let state: QueueState = { status: "loading" };
const listeners = new Set<() => void>();

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

/** Asks the server for the queue now. */
export function refreshQueue(): void {
  const room = watched;
  if (!room) return;
  loadQueue(room).then(
    (queue) => {
      if (watched === room) set({ status: "ready", queue });
    },
    (failure: unknown) => {
      if (watched === room && state.status !== "ready") {
        set({ status: "failed", message: errorMessage(failure) });
      }
    },
  );
}

/**
 * Starts keeping one room's queue current, and returns the function that stops it. One room at a
 * time: the room page calls this in an effect.
 */
export function watchQueue(room: WatchedRoom): () => void {
  watched = room;
  set({ status: "loading" });
  refreshQueue();
  return () => {
    if (watched !== room) return;
    watched = null;
    set({ status: "loading" });
  };
}

/** The room being watched, or null. The chat's cards build their links from it. */
export function watchedRoom(): WatchedRoom | null {
  return watched;
}

/** Takes the queue the server answered an action of this page with. */
export function acceptQueue(queue: Queue): void {
  if (watched) set({ status: "ready", queue });
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
