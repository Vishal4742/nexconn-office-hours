// Every call the room makes about sessions and hands. Each one that changes something answers
// with the queue as it is afterwards: give that to acceptQueue (store.ts) and the page is current
// without waiting for the ping. This file is a contract (notes/phase-2-plan.md).
import { ApiError, api } from "../api";
import { roomPath } from "../projects";
import type { RepoName } from "../projects";
import type { Queue } from "./types";

function asQueue(data: unknown): Queue {
  const queue = data as Partial<Queue> | null;
  const isQueue =
    typeof queue === "object" &&
    queue !== null &&
    Array.isArray(queue.hands) &&
    Array.isArray(queue.done) &&
    (queue.session === null || typeof queue.session === "object") &&
    typeof queue.serverTime === "string";
  if (!isQueue) {
    throw new ApiError(
      200,
      "The server sent something unexpected. Reload the page and try again.",
    );
  }
  // A server from before removedHandIds sends none.
  return { removedHandIds: [], ...queue } as Queue;
}

const ofProject = (repo: RepoName) => `/api${roomPath(repo)}`;
const ofSession = (id: string) => `/api/sessions/${encodeURIComponent(id)}`;
const ofHand = (id: string) => `/api/hands/${encodeURIComponent(id)}`;

async function act(path: string, body?: unknown): Promise<Queue> {
  return asQueue(await api.post<unknown>(path, body));
}

/** FR-QUEUE-4, FR-QUEUE-6: the session and its queue. Someone outside the room gets no hands. */
export async function loadQueue(repo: RepoName): Promise<Queue> {
  return asQueue(await api.get<unknown>(`${ofProject(repo)}/queue`));
}

export type NewSession = {
  /** 5 to 80 characters. */
  readonly title: string;
  /** 15 to 180, or 0 when no length is planned. */
  readonly plannedMinutes: number;
};

/** FR-SESS-1 (maintainer). */
export const startSession = (repo: RepoName, session: NewSession) =>
  act(`${ofProject(repo)}/sessions`, session);

/** FR-SESS-5 (maintainer): when the next session is planned; null clears it. */
export const setNextSession = (repo: RepoName, at: Date | null) =>
  act(`${ofProject(repo)}/next-session`, { at: at ? at.toISOString() : "" });

/** FR-SESS-4 (maintainer): carry sends the waiting hands to the next session. */
export const endSession = (sessionId: string, carry: boolean) =>
  act(`${ofSession(sessionId)}/end`, { carry });

/** FR-SESS-6 (maintainer): closes new hands, or opens them again. */
export const setLastCall = (sessionId: string, closed: boolean) =>
  act(`${ofSession(sessionId)}/lastcall`, { closed });

export type NewHand = {
  /** 10 to 120 characters. */
  readonly title: string;
  /** At most 2,000 characters; may be "". */
  readonly details: string;
};

/** FR-HAND-1. */
export const raiseHand = (sessionId: string, hand: NewHand) =>
  act(`${ofSession(sessionId)}/hands`, hand);

/** FR-HAND-4 (the author). */
export const withdrawHand = (handId: string) =>
  act(`${ofHand(handId)}/withdraw`);

/** FR-HAND-7: adds my +1 to someone else's waiting hand, or takes it back. */
export const togglePlus = (handId: string) => act(`${ofHand(handId)}/plus`);

/** FR-QUEUE-9 (the author of the hand being answered). */
export const imHere = (handId: string) => act(`${ofHand(handId)}/here`);

/** FR-QUEUE-3 (maintainer): takes the first waiting hand. */
export const nextHand = (sessionId: string) =>
  act(`${ofSession(sessionId)}/next`);

/** FR-QUEUE-3 (maintainer). */
export const skipHand = (handId: string) => act(`${ofHand(handId)}/skip`);

/** FR-MOD-1 (maintainer). */
export const removeHand = (handId: string) => act(`${ofHand(handId)}/remove`);

/** FR-QUEUE-8 (maintainer): the author of the hand being answered is not there. */
export const markAway = (handId: string) => act(`${ofHand(handId)}/away`);

/** FR-HAND-8 (maintainer): a waiting hand joins the one being answered. */
export const mergeHand = (handId: string) => act(`${ofHand(handId)}/merge`);

export type Answer = {
  /** 20 to 2,000 characters. */
  readonly answer: string;
  /** At most 5 http or https addresses. */
  readonly links: readonly string[];
};

/** FR-FAQ-1 (maintainer): finishes the hand being answered; the answer becomes a FAQ entry. */
export const finishHand = (handId: string, answer: Answer) =>
  act(`${ofHand(handId)}/done`, answer);

/** FR-MOD-1 (maintainer): the member cannot post in the room for this many minutes. */
export async function muteMember(
  repo: RepoName,
  userId: string,
  minutes = 10,
): Promise<void> {
  await api.post<unknown>(`${ofProject(repo)}/mute`, { userId, minutes });
}
