import { useSyncExternalStore } from "react";

import { ApiError, api } from "./api";

/** The signed-in person, as GET /api/me describes them. */
export type Me = {
  readonly id: string;
  readonly login: string;
  readonly name: string;
  readonly avatarUrl: string;
};

export type Auth =
  | { readonly status: "loading" }
  | { readonly status: "signedOut" }
  | { readonly status: "signedIn"; readonly me: Me }
  | { readonly status: "unreachable"; readonly message: string };

/** FR-AUTH-1: sign-in starts on the server and comes back to `next`. */
export function signInHref(next: string): string {
  return `/auth/github/login?next=${encodeURIComponent(next)}`;
}

const SIGN_IN_FAILURES: Readonly<Record<string, string>> = {
  cancelled: "You cancelled the GitHub sign-in. Nothing was changed.",
  expired:
    "That sign-in took too long or was finished in another tab. Try again.",
  github: "GitHub didn't complete the sign-in. Try again in a moment.",
  server: "We couldn't save your sign-in. Try again in a moment.",
  setup: "Sign-in isn't set up on this server yet.",
};

/** The sentence for a failed sign-in (the server redirects to /?signin=reason), or null. */
export function signInMessage(search: string): string | null {
  const reason = new URLSearchParams(search).get("signin");
  if (reason === null) return null;
  return SIGN_IN_FAILURES[reason] ?? "The sign-in didn't finish. Try again.";
}

/**
 * Where Try again after a failed sign-in leads: the page it started from, which the server sends
 * as `next` (state-machine audit, bug 10). Only a path of this site; the server checks it again.
 */
export function signInRetryPath(search: string): string {
  const next = new URLSearchParams(search).get("next") ?? "";
  const isOurs =
    next.startsWith("/") && !next.startsWith("//") && !next.includes("\\");
  return isOurs ? next : "/dashboard";
}

function isMe(value: unknown): value is Me {
  if (typeof value !== "object" || value === null) return false;
  const me = value as Record<string, unknown>;
  return (
    typeof me.id === "string" &&
    typeof me.login === "string" &&
    typeof me.name === "string" &&
    typeof me.avatarUrl === "string"
  );
}

let state: Auth = { status: "loading" };
let hasStarted = false;
const listeners = new Set<() => void>();

function set(next: Auth): void {
  state = next;
  listeners.forEach((listener) => listener());
}

/** Whom a state shows: a user id, null when signed out, undefined while it is not known. */
function shownUser(auth: Auth): string | null | undefined {
  if (auth.status === "signedIn") return auth.me.id;
  return auth.status === "signedOut" ? null : undefined;
}

/**
 * Whether a tab that shows `mine` must reload, now that the browser's session names `theirs`
 * (a user id, or null when signed out). The session cookie is one per browser: after a sign-out
 * or another account's sign-in in another tab, this tab's calls already go out as someone else
 * while it still shows the person before, and its chat stays connected as them (state-machine
 * audit, bug 5). Only a tab that knows whom it shows reloads.
 */
export function mustReload(mine: Auth, theirs: string | null): boolean {
  const shown = shownUser(mine);
  return shown !== undefined && shown !== theirs;
}

/** Who the browser's session names now. */
async function ask(): Promise<Auth> {
  try {
    const me = await api.get<unknown>("/api/me");
    return isMe(me) ? { status: "signedIn", me } : { status: "signedOut" };
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      return { status: "signedOut" };
    }
    return {
      status: "unreachable",
      message:
        error instanceof Error ? error.message : "Can't reach the server.",
    };
  }
}

const CHANNEL = "office-hours-auth";
let tabs: BroadcastChannel | null = null;
// Whether a load is on its way, and whether another tab spoke meanwhile: that load's answer may
// be from before the other tab's sign-in or sign-out, so it is asked again.
let isLoading = false;
let heardWhileLoading = false;

/** Tells the other tabs of this site whom this one now knows the session names. */
function tellOtherTabs(): void {
  const user = shownUser(state);
  if (user !== undefined) tabs?.postMessage({ user });
}

function reloadIfSomeoneElse(theirs: string | null): void {
  if (mustReload(state, theirs)) window.location.reload();
}

/**
 * Follows the session across tabs: another tab says whom it found, at once; a tab that comes
 * back into view asks the server, for a sign-in no tab of ours saw (the agent consent page) and
 * for browsers without BroadcastChannel.
 */
function followOtherTabs(): void {
  if (typeof BroadcastChannel !== "undefined") {
    tabs = new BroadcastChannel(CHANNEL);
    tabs.onmessage = (event: MessageEvent<{ readonly user?: unknown }>) => {
      const { user } = event.data ?? {};
      if (user !== null && typeof user !== "string") return;
      if (isLoading) heardWhileLoading = true;
      else reloadIfSomeoneElse(user);
    };
  }
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") return;
    if (shownUser(state) === undefined) return;
    void ask().then((now) => {
      const theirs = shownUser(now);
      if (theirs !== undefined) reloadIfSomeoneElse(theirs);
    });
  });
}

async function load(): Promise<void> {
  isLoading = true;
  let now = await ask();
  // Asked again until an answer comes back with no other tab speaking meanwhile.
  while (heardWhileLoading) {
    heardWhileLoading = false;
    now = await ask();
  }
  isLoading = false;
  set(now);
  tellOtherTabs();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  if (!hasStarted) {
    hasStarted = true;
    followOtherTabs();
    void load();
  }
  return () => {
    listeners.delete(listener);
  };
}

/** Who is signed in. The first caller starts the one request that finds out. */
export function useAuth(): Auth {
  return useSyncExternalStore(subscribe, () => state);
}

/** Asks again after the server could not be reached. */
export function retryAuth(): void {
  set({ status: "loading" });
  void load();
}

/** FR-AUTH-4: clears the session on the server, then here, then in the other tabs. */
export async function signOut(): Promise<void> {
  await api.post("/auth/logout");
  set({ status: "signedOut" });
  tellOtherTabs();
}
