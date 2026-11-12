import { useEffect, useRef } from "react";

import { Check } from "../components/Icons";
import { imHere } from "../queue/api";
import type { Hand, Queue } from "../queue/types";
import { Refused, useQueueSend } from "./sheets/parts";

type Props = {
  /** Null until the queue has loaded. */
  readonly queue: Queue | null;
};

/** Who is answering, as the banner names them. */
function answerer(hand: Hand): string {
  return hand.takenBy ? `@${hand.takenBy.login}` : "A maintainer";
}

/**
 * "It's your turn" (FR-QUEUE-5, FR-QUEUE-9): shown while my hand is the one being answered, with
 * the "I'm here" button. It follows the queue the server returned, never a message alone: cards
 * and pings are replayed after a reconnect. The props are a contract (notes/phase-2-plan.md).
 */
export function YourTurnBanner({ queue }: Props) {
  const hand =
    queue?.hands.find((one) => one.isMine && one.status === "answering") ??
    null;
  return (
    <>
      {/* Always in the page, so a screen reader hears the turn the moment it comes (NFR-A11Y-1). */}
      <p className="sr-only" aria-live="assertive">
        {hand
          ? `It's your turn. ${answerer(hand)} is answering your question.`
          : ""}
      </p>
      {hand && <Banner key={hand.id} hand={hand} />}
    </>
  );
}

function Banner({ hand }: { readonly hand: Hand }) {
  const [sending, send] = useQueueSend();
  const messageRef = useRef<HTMLParagraphElement>(null);
  const isHere = hand.ackedAt !== null;
  const wasHere = useRef(isHere);
  const isSending = sending.status === "sending";
  const who = answerer(hand);

  useEffect(() => {
    // Once they know, the button is gone: keep the keyboard's place on the banner, not the page.
    if (
      isHere &&
      !wasHere.current &&
      document.activeElement === document.body
    ) {
      messageRef.current?.focus();
    }
    wasHere.current = isHere;
  }, [isHere]);

  return (
    <div className="turn-banner">
      <svg
        className="turn-hand"
        viewBox="0 0 24 24"
        aria-hidden="true"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M8 15.5V6.5a1.5 1.5 0 0 1 3 0v5M11 11.5v-7a1.5 1.5 0 0 1 3 0v7M14 11.5v-6a1.5 1.5 0 0 1 3 0v6" />
        <path d="M17 11.5v-3a1.5 1.5 0 0 1 3 0V14a7 7 0 0 1-7 7h-1a6 6 0 0 1-4.8-2.4l-3.3-3.2a1.5 1.5 0 0 1 2.4-1.8L8 15.5" />
      </svg>
      <div className="turn-text">
        <p className="turn-title">It's your turn</p>
        <p className="turn-message" ref={messageRef} tabIndex={-1}>
          {isHere && <Check />}
          {isHere
            ? `${who} knows you're here.`
            : `${who} is answering your question.`}
        </p>
      </div>
      {!isHere && (
        <button
          type="button"
          className="btn turn-btn"
          disabled={isSending}
          onClick={() => void send(() => imHere(hand.id))}
        >
          {isSending ? "Telling them…" : "I'm here"}
        </button>
      )}
      <Refused sending={sending} />
    </div>
  );
}
