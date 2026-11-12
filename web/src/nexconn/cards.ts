// What the three cards Office Hours posts into a room say: OH:Session, OH:Hand and OH:FAQ. This
// file has no DOM in it, so it can be tested as plain logic; cardElements.ts draws the cards.
//
// Everything read out of a message is untrusted (NFR-SEC-6): each field is checked for its type
// and its shape, and a field that fails shows nothing. server/notify.go has the payloads.
import { faqPath } from "../faq";
import { issueUrl } from "../issueLink";
import type { WatchedRoom } from "../queue/store";
import type { Hand, HandStatus } from "../queue/types";

/** Every OH:* message is sent by this Nexconn user. */
export const BOT_ID = "oh_bot";

/** FR-QUEUE-7: the one sender check. Call it before acting on an OH:* message or showing one. */
export function isFromBot(message: { readonly senderUserId: string }): boolean {
  return message.senderUserId === BOT_ID;
}

export type CardMessage = {
  readonly senderUserId: string;
  readonly messageType: string;
  readonly content: unknown;
  /** The room the message was sent in: Nexconn's group. */
  readonly channelIdentifier?: { readonly channelId: string };
};

export type CardLine =
  | {
      readonly kind: "eyebrow" | "title" | "muted";
      readonly text: string;
    }
  /** Where a hand stands now. The status is also how the line is coloured. */
  | {
      readonly kind: "status";
      readonly text: string;
      readonly status: HandStatus;
    };

/** What a button of the FAQ card does to the removal of its entry. */
export type RemovalMove = "ask" | "keep" | "remove";

export type CardAction =
  /** Opens in a new tab. The address is built here, never read from a message. */
  | {
      readonly kind: "link";
      readonly text: string;
      readonly href: string;
      readonly rel: string;
    }
  | {
      readonly kind: "button";
      readonly text: string;
      readonly does: RemovalMove;
    };

export type Card = {
  readonly lines: readonly CardLine[];
  readonly actions: readonly CardAction[];
  /** What the card's own button led to, for a live region. "" when there is nothing to say. */
  readonly said: string;
};

/** Where "Remove from the FAQ" stands for one entry (FR-FAQ-6). */
export type Removal =
  | { readonly step: "idle" | "asking" | "sending" | "removed" }
  | { readonly step: "refused"; readonly message: string };

/** What a card is read against. It is handed in, so that this file stays plain logic. */
export type CardContext = {
  /** The room whose queue is watched (watchedRoom of the queue store), or null. */
  readonly room: WatchedRoom | null;
  /** A hand of that room's session by its id (findHand of the queue store), or null. */
  readonly findHand: (handId: string) => Hand | null;
  /** This site's origin: the answer's full address goes into the text of a GitHub issue. */
  readonly origin: string;
  /** Where the removal of this card's entry stands. Only a FAQ card reads it. */
  readonly removal: Removal;
};

const SLUG = /^[a-z0-9-]{1,80}$/;
const LOGIN = /^[A-Za-z0-9-]{1,39}$/;
// An id is a whole number written as text (idText in server/notify.go).
const ID = /^[1-9][0-9]{0,18}$/;
// More than any title the server accepts (120 characters), with room for the characters that
// count twice in a string's length.
const MAX_TITLE = 300;

const NOTHING: Card = { lines: [], actions: [], said: "" };
const IGNORED: Card = {
  ...NOTHING,
  lines: [
    { kind: "muted", text: "Ignored: this card was not sent by Office Hours." },
  ],
};

const STATUS_TEXT: Readonly<Record<HandStatus, string>> = {
  waiting: "In line",
  answering: "Being answered now",
  answered: "Answered",
  skipped: "Skipped",
  withdrawn: "Withdrawn",
  removed: "Removed by a maintainer",
  not_reached: "Not reached",
  carried: "Carried to the next session",
  merged: "Merged into another question",
};

function field(content: unknown, key: string): unknown {
  if (typeof content !== "object" || content === null) return undefined;
  return (content as Readonly<Record<string, unknown>>)[key];
}

function isSlug(value: unknown): value is string {
  return typeof value === "string" && SLUG.test(value);
}

/** A text field of exactly this shape, or "". */
function shaped(content: unknown, key: string, shape: RegExp): string {
  const value = field(content, key);
  return typeof value === "string" && shape.test(value) ? value : "";
}

/** The title, trimmed, or "" when it is not text or far longer than any title can be. */
function titleOf(content: unknown): string {
  const value = field(content, "title");
  if (typeof value !== "string" || value.length > MAX_TITLE) return "";
  return value.trim();
}

/** A whole number from 0 up. Anything else counts as nothing. */
function count(content: unknown, key: string): number {
  const value = field(content, key);
  const isCount =
    typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
  return isCount ? value : 0;
}

/** One line of the card, or none when there is nothing to say in it. */
function line(
  kind: "eyebrow" | "title" | "muted",
  text: string,
): readonly CardLine[] {
  return text ? [{ kind, text }] : [];
}

/** "9 answered · 2 carried over · 1 not reached", without what is 0 (FR-SESS-4, FR-SESS-7). */
function tally(content: unknown): string {
  const counts: readonly (readonly [number, string])[] = [
    [count(content, "answered"), "answered"],
    [count(content, "carried"), "carried over"],
    [count(content, "notReached"), "not reached"],
  ];
  return counts
    .filter(([n]) => n > 0)
    .map(([n, what]) => `${n} ${what}`)
    .join(" · ");
}

function sessionEyebrow(state: unknown): string {
  if (state === "live") return "Office hours started";
  if (state === "ended") return "Office hours ended";
  return "Office hours";
}

function sessionCard(content: unknown): Card {
  const state = field(content, "state");
  return {
    ...NOTHING,
    lines: [
      ...line("eyebrow", sessionEyebrow(state)),
      ...line("title", titleOf(content)),
      ...line("muted", state === "ended" ? tally(content) : ""),
    ],
  };
}

/** The answer's page: on our own origin, from the watched room's name and a checked slug. */
function answerLink(room: WatchedRoom, slug: string): CardAction {
  return {
    kind: "link",
    text: "Read the answer",
    href: faqPath(room, slug),
    rel: "noopener",
  };
}

/** A hand's status as the queue has it now. A status this page does not know says nothing. */
function statusLine(hand: Hand | null): readonly CardLine[] {
  const isKnown =
    hand !== null &&
    Object.prototype.hasOwnProperty.call(STATUS_TEXT, hand.status);
  if (!isKnown) return [];
  return [
    { kind: "status", text: STATUS_TEXT[hand.status], status: hand.status },
  ];
}

/**
 * FR-HAND-5: a card cannot change once it is sent, so what became of the hand is read from the
 * queue by its id. A hand the queue no longer holds (an older session's) shows no status.
 */
function handCard(content: unknown, context: CardContext): Card {
  const handId = shaped(content, "handId", ID);
  const login = shaped(content, "authorLogin", LOGIN);
  const hand = handId ? context.findHand(handId) : null;
  const isAnswered = hand?.status === "answered" && isSlug(hand.faqSlug);
  return {
    ...NOTHING,
    lines: [
      ...line("muted", login ? `@${login} raised a hand` : "A hand was raised"),
      // A maintainer took a removed hand's words out of the room; the queue sends none either.
      ...line("title", hand?.status === "removed" ? "" : titleOf(content)),
      ...statusLine(hand),
    ],
    actions:
      hand && isAnswered && context.room
        ? [answerLink(context.room, hand.faqSlug)]
        : [],
  };
}

/** FR-HAND-8: "Asked by @a, @b and 2 more". Two names once there are more than three people. */
function askedBy(value: unknown): string {
  if (!Array.isArray(value)) return "";
  const logins = [
    ...new Set(
      value.filter(
        (login): login is string =>
          typeof login === "string" && LOGIN.test(login),
      ),
    ),
  ].map((login) => `@${login}`);
  if (logins.length === 0) return "";
  if (logins.length === 1) return `Asked by ${logins[0]}`;
  if (logins.length > 3) {
    return `Asked by ${logins[0]}, ${logins[1]} and ${logins.length - 2} more`;
  }
  const last = logins[logins.length - 1];
  return `Asked by ${logins.slice(0, -1).join(", ")} and ${last}`;
}

/**
 * FR-FAQ-5: GitHub's "new issue" page of the room's repo, filled in. Who asked stays in the room
 * (FR-HAND-8), so the issue carries the question and where its answer is, and no names.
 */
function issueLink(
  room: WatchedRoom,
  title: string,
  slug: string,
  origin: string,
): CardAction {
  const body = slug
    ? `This question was answered in office hours:\n${origin}${faqPath(room, slug)}`
    : "This question was answered in office hours.";
  return {
    kind: "link",
    text: "Open a GitHub issue",
    href: issueUrl(room, title, body),
    rel: "noopener noreferrer nofollow",
  };
}

/** Whether a message was sent in the watched room. Only then may a link use that room's name. */
function isOfRoom(
  message: CardMessage,
  room: WatchedRoom | null,
): room is WatchedRoom {
  const channelId = message.channelIdentifier?.channelId;
  return (
    room !== null &&
    typeof channelId === "string" &&
    channelId !== "" &&
    channelId === room.groupId
  );
}

function button(text: string, does: RemovalMove): CardAction {
  return { kind: "button", text, does };
}

/** FR-FAQ-6: the card with "Remove from the FAQ" at the step it is on. */
function removable(
  lines: readonly CardLine[],
  links: readonly CardAction[],
  removal: Removal,
): Card {
  switch (removal.step) {
    case "asking":
      return {
        lines: [
          ...lines,
          ...line(
            "muted",
            "Remove this answer from the FAQ? Its page will say that it was removed.",
          ),
        ],
        // The safe answer comes first: it is the one that gets the keyboard's focus.
        actions: [button("Keep it", "keep"), button("Remove it", "remove")],
        said: "",
      };
    case "sending":
      return { lines, actions: [], said: "Removing…" };
    case "removed":
      return {
        lines,
        actions: [],
        said: "Removed from the FAQ. Its page now says that it was removed.",
      };
    case "idle":
    case "refused":
      return {
        lines,
        actions: [...links, button("Remove from the FAQ", "ask")],
        said: removal.step === "refused" ? removal.message : "",
      };
  }
}

/**
 * FR-FAQ-2: the link to the answer is built from the slug, on our own origin, and only for a
 * card of the watched room: its name is what the address is made of.
 */
function faqCard(message: CardMessage, context: CardContext): Card {
  const { content } = message;
  const title = titleOf(content);
  const lines = [
    ...line("eyebrow", "Added to FAQ"),
    ...line("title", title),
    ...line("muted", askedBy(field(content, "askedBy"))),
  ];
  if (!isOfRoom(message, context.room)) return { ...NOTHING, lines };

  const { room } = context;
  const slug = shaped(content, "slug", SLUG);
  const links = [
    ...(slug ? [answerLink(room, slug)] : []),
    ...(title ? [issueLink(room, title, slug, context.origin)] : []),
  ];
  // The server checks again who may delete (NFR-SEC-2); this only decides what is offered.
  const canRemove = room.isMaintainer && shaped(content, "faqId", ID) !== "";
  if (!canRemove) return { ...NOTHING, lines, actions: links };
  return removable(lines, links, context.removal);
}

/** The entry a FAQ card of the bot is about: what deleteFaq takes. "" for anything else. */
export function faqIdOf(message: CardMessage): string {
  const isFaqCard = isFromBot(message) && message.messageType === "OH:FAQ";
  return isFaqCard ? shaped(message.content, "faqId", ID) : "";
}

/**
 * What a card shows. A card that the bot did not send shows none of its content (FR-QUEUE-7),
 * and every address is built here: one inside a message is never used (NFR-SEC-6).
 */
export function describeCard(message: CardMessage, context: CardContext): Card {
  if (!isFromBot(message)) return IGNORED;
  switch (message.messageType) {
    case "OH:Session":
      return sessionCard(message.content);
    case "OH:Hand":
      return handCard(message.content, context);
    case "OH:FAQ":
      return faqCard(message, context);
    default:
      return NOTHING;
  }
}
