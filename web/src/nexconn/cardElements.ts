// The elements Chat UI puts inside a message bubble for our three card types. It creates one and
// sets `.message` on it. What a card says is decided in cards.ts.

import { cardLines, type CardMessage } from "./cards";

const CARD_CSS = `
  :host { display: block; min-width: 220px; max-width: 420px; font-size: 14px; line-height: 20px; }
  .eyebrow { font-size: 12px; line-height: 16px; letter-spacing: .06em; text-transform: uppercase; color: #0047ff; }
  .title { margin-top: 4px; font-size: 16px; line-height: 24px; font-weight: 500; }
  .muted { color: #6b6d76; }
`;

class OhCard extends HTMLElement {
  #message: CardMessage | null = null;
  readonly #root = this.attachShadow({ mode: "open" });

  get message(): CardMessage | null {
    return this.#message;
  }

  set message(value: CardMessage | null) {
    this.#message = value;
    this.render();
  }

  connectedCallback(): void {
    this.render();
  }

  private render(): void {
    const style = document.createElement("style");
    style.textContent = CARD_CSS;
    const lines = this.#message ? cardLines(this.#message) : [];
    this.#root.replaceChildren(
      style,
      ...lines.map((line) => {
        const el = document.createElement("div");
        el.className = line.kind;
        el.textContent = line.text; // text, never HTML
        return el;
      }),
    );
  }
}

// One class per tag: a custom element class can be registered under one name only.
export class OhSessionCard extends OhCard {}
export class OhHandCard extends OhCard {}
export class OhFaqCard extends OhCard {}
