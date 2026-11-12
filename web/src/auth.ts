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

async function load(): Promise<void> {
  try {
    const me = await api.get<unknown>("/api/me");
    set(isMe(me) ? { status: "signedIn", me } : { status: "signedOut" });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      set({ status: "signedOut" });
      return;
    }
    set({
      status: "unreachable",
      message:
        error instanceof Error ? error.message : "Can't reach the server.",
    });
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  if (!hasStarted) {
    hasStarted = true;
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

/** FR-AUTH-4: clears the session on the server, then here. */
export async function signOut(): Promise<void> {
  await api.post("/auth/logout");
  set({ status: "signedOut" });
}
