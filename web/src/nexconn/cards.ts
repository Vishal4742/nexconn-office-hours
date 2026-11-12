// What the three cards Office Hours posts into a room say: OH:Session, OH:Hand and OH:FAQ. This
// file has no DOM in it, so it can be tested as plain logic; cardElements.ts draws the cards.

/** Every OH:* message is sent by this Nexconn user. */
export const BOT_ID = "oh_bot";

/** FR-QUEUE-7: the one sender check. Call it before acting on an OH:* message or showing one. */
export function isFromBot(message: { readonly senderUserId: string }): boolean {
  return message.senderUserId === BOT_ID;
}

export type CardLine = {
  readonly kind: "eyebrow" | "title" | "muted";
  readonly text: string;
};

export type CardMessage = {
  readonly senderUserId: string;
  readonly messageType: string;
  readonly content: unknown;
};

/** Reads a string out of untrusted message content; anything else becomes "". */
function field(content: unknown, key: string): string {
  if (typeof content !== "object" || content === null) return "";
  const value = (content as Record<string, unknown>)[key];
  return typeof value === "string" ? value : "";
}

/**
 * What a card shows, as plain text lines (NFR-SEC-6). A card that the bot did not send shows none
 * of its content, and the FAQ link is built from the slug: a URL inside a card is never used.
 */
export function cardLines(message: CardMessage): readonly CardLine[] {
  if (!isFromBot(message)) {
    return [
      {
        kind: "muted",
        text: "Ignored: this card was not sent by Office Hours.",
      },
    ];
  }
  const { content } = message;
  switch (message.messageType) {
    case "OH:Session":
      return [
        {
          kind: "eyebrow",
          text:
            field(content, "state") === "ended"
              ? "Office hours ended"
              : "Office hours started",
        },
        { kind: "title", text: field(content, "title") },
      ];
    case "OH:Hand":
      return [
        {
          kind: "muted",
          text: `@${field(content, "authorLogin")} raised a hand`,
        },
        { kind: "title", text: field(content, "title") },
      ];
    case "OH:FAQ":
      return [
        { kind: "eyebrow", text: "Added to FAQ" },
        { kind: "title", text: field(content, "title") },
        { kind: "muted", text: `/faq/${field(content, "slug")}` },
      ];
    default:
      return [];
  }
}
