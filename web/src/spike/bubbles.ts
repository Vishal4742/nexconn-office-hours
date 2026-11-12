import type { ChatUIMessageModel } from "@nexconn/chatui";

/** Every OH:* message is sent by this Nexconn user. Cards from anyone else are forgeries (NFR-SEC-6). */
export const BOT_ID = "oh_bot";

/** The one sender check: call it before acting on an OH:* message or rendering one. */
export function isFromBot(message: { readonly senderUserId: string }): boolean {
  return message.senderUserId === BOT_ID;
}

const CARD_CSS = `
  :host { display: block; min-width: 220px; max-width: 420px; font-size: 14px; line-height: 20px; }
  .eyebrow { font-size: 12px; line-height: 16px; letter-spacing: .06em; text-transform: uppercase; color: #0047ff; }
  .title { margin-top: 4px; font-size: 16px; line-height: 24px; font-weight: 500; }
  .muted { color: #6b6d76; }
`;

/** Reads a string from untrusted message content; anything else becomes "". */
function field(content: Record<string, unknown>, key: string): string {
  const value = content[key];
  return typeof value === "string" ? value : "";
}

function line(className: string, text: string): HTMLElement {
  const el = document.createElement("div");
  el.className = className;
  el.textContent = text; // user text is always rendered as text
  return el;
}

/** Base for our message cards. Chat UI sets `.message` on the element it creates for a bubble. */
abstract class OhCard extends HTMLElement {
  #message: ChatUIMessageModel | null = null;
  readonly #root = this.attachShadow({ mode: "open" });

  get message(): ChatUIMessageModel | null {
    return this.#message;
  }

  set message(value: ChatUIMessageModel | null) {
    this.#message = value;
    this.render();
  }

  connectedCallback(): void {
    this.render();
  }

  private render(): void {
    const style = document.createElement("style");
    style.textContent = CARD_CSS;
    const message = this.#message;
    if (!message) {
      this.#root.replaceChildren(style);
      return;
    }
    const lines = isFromBot(message)
      ? this.lines(message.content)
      : [line("muted", "Ignored: this card was not sent by Office Hours.")];
    this.#root.replaceChildren(style, ...lines);
  }

  protected abstract lines(content: Record<string, unknown>): HTMLElement[];
}

export class OhSessionCard extends OhCard {
  protected lines(content: Record<string, unknown>): HTMLElement[] {
    const live = field(content, "state") !== "ended";
    return [
      line("eyebrow", live ? "Office hours started" : "Office hours ended"),
      line("title", field(content, "title")),
    ];
  }
}

export class OhHandCard extends OhCard {
  protected lines(content: Record<string, unknown>): HTMLElement[] {
    return [
      line("muted", `@${field(content, "authorLogin")} raised a hand`),
      line("title", field(content, "title")),
    ];
  }
}

export class OhFaqCard extends OhCard {
  protected lines(content: Record<string, unknown>): HTMLElement[] {
    return [
      line("eyebrow", "Added to FAQ"),
      line("title", field(content, "title")),
      // The link is built from the slug on our own origin, never from a URL in the payload.
      line("muted", `/faq/${field(content, "slug")}`),
    ];
  }
}
