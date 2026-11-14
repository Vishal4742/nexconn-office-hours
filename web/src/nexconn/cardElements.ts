// The elements Chat UI puts inside a message bubble for our three card types. It creates one and
// sets `.message` on it, and it may hand the same element another message later: its list reuses
// elements by position. What a card says is decided in cards.ts; this file draws it and does what
// its buttons ask.

import { errorMessage } from "../api";
import { deleteFaq } from "../faq";
import {
  findHand,
  refreshQueue,
  subscribeQueue,
  wasRemoved,
  watchedRoom,
} from "../queue/store";
import { describeCard, faqIdOf } from "./cards";
import type {
  Card,
  CardAction,
  CardLine,
  CardMessage,
  Removal,
  RemovalMove,
} from "./cards";

// A card sits inside Chat UI's bubble, which gives it its colour and corners: no border or
// background here. Page styles don't reach inside the element; the tokens do, as inherited
// custom properties. Muted text is the room's darker one: 6.4:1 on the bubble's #f1f0ec.
const CARD_CSS = `
  :host { --card-muted: #54565e; display: block; max-width: 420px; padding: 16px 20px; font-size: 14px; line-height: 20px; overflow-wrap: anywhere; }
  p { margin: 0; }
  .body > * + * { margin-top: 4px; }
  .eyebrow { font: 500 12px/16px var(--mono, ui-monospace, monospace); letter-spacing: .06em; text-transform: uppercase; color: var(--blue, #0047ff); }
  .title { font-size: 16px; line-height: 24px; font-weight: 500; }
  .muted { color: var(--card-muted); }
  .body > .status { width: fit-content; margin-top: 8px; padding: 1px 8px; border: 1px solid currentColor; border-radius: 999px; font: 500 12px/16px var(--mono, ui-monospace, monospace); color: var(--card-muted); }
  .status[data-status="answering"] { background: var(--blue, #0047ff); border-color: var(--blue, #0047ff); color: #fff; }
  .status[data-status="answered"] { color: var(--green, #1a7a43); }
  .status[data-status="skipped"] { color: var(--amber, #8f5e12); }
  .body > .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; margin-top: 12px; }
  a, button { font: inherit; font-weight: 600; border-radius: 4px; }
  a { color: var(--blue, #0047ff); text-decoration: none; }
  a:hover, .ask:hover { text-decoration: underline; text-underline-offset: 3px; }
  button { padding: 0; border: 0; background: none; color: inherit; cursor: pointer; }
  .ask { color: var(--card-muted); }
  .keep, .remove { padding: 4px 12px; border: 1px solid; border-radius: 999px; }
  .keep { border-color: var(--divider, #c8c4b8); background: #fff; }
  .remove { color: var(--amber, #8f5e12); }
  :focus-visible { outline: 2px solid var(--blue, #0047ff); outline-offset: 2px; }
  .said:not(:empty) { margin-top: 8px; color: var(--card-muted); }
  .said.err { color: var(--amber, #8f5e12); font-weight: 500; }
`;

const IDLE: Removal = { step: "idle" };

// "Remove from the FAQ" per entry, for as long as the page lives. It is kept here and not on an
// element: an entry's card can move to another element at any time (once the list holds 300
// messages, every new one moves all the others by one place).
const removals = new Map<string, Removal>();
const redraws = new Set<() => void>();

function setRemoval(faqId: string, removal: Removal): void {
  removals.set(faqId, removal);
  redraws.forEach((redraw) => redraw());
}

/** FR-FAQ-6. The server checks again that the caller maintains the entry's project. */
async function removeEntry(faqId: string): Promise<void> {
  setRemoval(faqId, { step: "sending" });
  try {
    await deleteFaq(faqId);
    setRemoval(faqId, { step: "removed" });
    refreshQueue(); // so an answered hand stops linking to the entry
  } catch (failure) {
    setRemoval(faqId, { step: "refused", message: errorMessage(failure) });
  }
}

function lineElement(line: CardLine): HTMLElement {
  const el = document.createElement("p");
  el.className = line.kind;
  if (line.kind === "status") el.dataset.status = line.status;
  el.textContent = line.text; // text, never HTML
  return el;
}

function actionElement(
  action: CardAction,
  onMove: (move: RemovalMove) => void,
): HTMLElement {
  if (action.kind === "button") {
    const button = document.createElement("button");
    button.type = "button";
    button.className = action.does;
    button.textContent = action.text;
    button.addEventListener("click", () => onMove(action.does));
    return button;
  }
  // The address was built in cards.ts from checked values; none comes from the message.
  const link = document.createElement("a");
  link.href = action.href;
  link.target = "_blank"; // the room stays open behind it
  link.rel = action.rel;
  const arrow = document.createElement("span");
  arrow.setAttribute("aria-hidden", "true");
  arrow.textContent = " ↗";
  link.append(action.text, arrow);
  return link;
}

class OhCard extends HTMLElement {
  #message: CardMessage | null = null;
  #drawn = "";
  #stopWatching: (() => void) | null = null;
  readonly #root = this.attachShadow({ mode: "open" });
  readonly #body = document.createElement("div");
  // Always in the card, so a screen reader hears what a button led to when the text changes.
  readonly #said = document.createElement("p");
  readonly #redraw = () => this.#render();

  constructor() {
    super();
    const style = document.createElement("style");
    style.textContent = CARD_CSS;
    this.#body.className = "body";
    this.#said.setAttribute("role", "status");
    this.#root.append(style, this.#body, this.#said);
  }

  get message(): CardMessage | null {
    return this.#message;
  }

  set message(value: CardMessage | null) {
    this.#message = value;
    this.#render();
  }

  // A hand card shows what became of its hand, and a FAQ card what the viewer may do: both follow
  // the queue store while the card is on the page.
  connectedCallback(): void {
    if (!this.#stopWatching) {
      const stopQueue = subscribeQueue(this.#redraw);
      redraws.add(this.#redraw);
      this.#stopWatching = () => {
        stopQueue();
        redraws.delete(this.#redraw);
      };
    }
    this.#render();
  }

  disconnectedCallback(): void {
    this.#stopWatching?.();
    this.#stopWatching = null;
  }

  #removal(): Removal {
    return (this.#message && removals.get(faqIdOf(this.#message))) || IDLE;
  }

  #card(): Card {
    if (!this.#message) return { lines: [], actions: [], said: "" };
    return describeCard(this.#message, {
      room: watchedRoom(),
      findHand,
      isRemoved: wasRemoved,
      origin: window.location.origin,
      removal: this.#removal(),
    });
  }

  #render(): void {
    const card = this.#card();
    const drawn = JSON.stringify(card);
    // Most queue changes are about other hands. Drawing again would take the keyboard's focus
    // off a link of this card for nothing.
    if (drawn === this.#drawn) return;
    this.#drawn = drawn;

    const lines = card.lines.map(lineElement);
    const actions = document.createElement("div");
    actions.className = "actions";
    actions.append(
      ...card.actions.map((action) =>
        actionElement(action, (move) => this.#move(move)),
      ),
    );
    this.#body.replaceChildren(
      ...lines,
      ...(card.actions.length > 0 ? [actions] : []),
    );
    this.#said.className =
      this.#removal().step === "refused" ? "said err" : "said";
    if (this.#said.textContent !== card.said) {
      this.#said.textContent = card.said;
    }
  }

  #move(move: RemovalMove): void {
    const faqId = this.#message ? faqIdOf(this.#message) : "";
    if (!faqId) return;
    if (move === "ask") setRemoval(faqId, { step: "asking" });
    if (move === "keep") setRemoval(faqId, IDLE);
    if (move === "remove") void removeEntry(faqId);
    // The pressed button is gone: the keyboard goes on from the first button there is now
    // ("Keep it" after asking, "Remove from the FAQ" after keeping).
    this.#body.querySelector("button")?.focus();
  }
}

// One class per tag: a custom element class can be registered under one name only.
export class OhSessionCard extends OhCard {}
export class OhHandCard extends OhCard {}
export class OhFaqCard extends OhCard {}
