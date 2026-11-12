// A session and its queue as the server describes them to one viewer (server/api_queue.go is the
// same shape). This file is a contract: the panel, the sheets, the room's head and the chat cards
// are built against it at the same time (notes/phase-2-plan.md), so it changes only through the
// plan. Ids are strings; times are ISO strings, or null when the thing has not happened.

/** Where a hand is in its life (FR-QUEUE-2). Open hands are "waiting" or "answering". */
export type HandStatus =
  | "waiting"
  | "answering"
  | "answered"
  | "skipped"
  | "withdrawn"
  | "removed"
  | "not_reached"
  | "carried"
  | "merged";

/** Someone as the queue shows them. The login is their GitHub name, written with an @. */
export type QueuePerson = {
  readonly userId: string;
  readonly login: string;
  readonly name: string;
  readonly avatarUrl: string;
};

export type QueueSession = {
  readonly id: string;
  /** 1 for the project's first session. */
  readonly number: number;
  readonly title: string;
  readonly live: boolean;
  /** Last call: no new hands (FR-SESS-6). */
  readonly handsClosed: boolean;
  readonly startedAt: string;
  readonly endedAt: string | null;
  /** 0 when no length was planned. */
  readonly plannedMinutes: number;
  /** Goes up by one with every change of the queue (FR-QUEUE-4). */
  readonly version: number;
  readonly host: QueuePerson | null;
};

export type Hand = {
  readonly id: string;
  /** "" for a hand a maintainer removed: its words are no longer shown. */
  readonly title: string;
  readonly details: string;
  readonly status: HandStatus;
  readonly author: QueuePerson;
  readonly raisedAt: string;
  readonly takenAt: string | null;
  readonly takenBy: QueuePerson | null;
  /** When the author pressed "I'm here" (FR-QUEUE-9). */
  readonly ackedAt: string | null;
  readonly closedAt: string | null;
  /** How often the author missed their turn (FR-QUEUE-8). */
  readonly awayCount: number;
  readonly plusCount: number;
  readonly plusByMe: boolean;
  readonly isMine: boolean;
  /** The FAQ entry of an answered hand, else "": what deleteFaq (faq.ts) takes. */
  readonly faqId: string;
  /** That entry's slug, else "". Build its address with faqPath (faq.ts). */
  readonly faqSlug: string;
};

export type Queue = {
  /** The live session, else the last one that ended, else null. */
  readonly session: QueueSession | null;
  /** Open hands in the server's order: the one being answered first, then the waiting ones. */
  readonly hands: readonly Hand[];
  /** The session's other hands, the latest first. */
  readonly done: readonly Hand[];
  /** FR-SESS-5. */
  readonly nextSessionAt: string | null;
  /** 0 until two hands were answered in this session (FR-QUEUE-10). */
  readonly medianAnswerSeconds: number;
  /** The server's clock when it answered: count minutes from this, not from the page's clock. */
  readonly serverTime: string;
};
