// The piece of product UI for each moment of the story, in the app's own look. Each is a picture
// with a description (role="img"): nothing in it can be pressed. The people and the repo are the
// same as in the app's queue fixtures.

import { ArrowUpRight, Bookmark, Bulb, RaisedHand } from "./icons";
import type { Moment as MomentKind } from "./story";

type Props = { readonly kind: MomentKind };

export function Moment({ kind }: Props) {
  switch (kind) {
    case "started":
      return <SessionStarted />;
    case "hand":
      return <HandGoesUp />;
    case "turn":
      return <YourTurn />;
    case "answered":
      return <AnsweredOnce />;
    case "ended":
      return <SessionEnded />;
  }
}

/** FR-SESS-3: the card that tells everyone in the room the session started. */
function SessionStarted() {
  return (
    <div
      className="card land-ui"
      role="img"
      aria-label="The card the room gets: Thursday office hours started, 45 minutes with @mia-k, for @all."
    >
      <p className="land-live">
        <span className="land-live-dot" />
        Started
      </p>
      <p className="land-ui-title">Thursday office hours</p>
      <p className="land-ui-meta">
        45 min with @mia-k · <span className="land-mention">@all</span>
      </p>
    </div>
  );
}

/** FR-HAND-1 and 3: the question in one line, and an answer to a similar one shown first. */
function HandGoesUp() {
  return (
    <div
      className="card land-ui"
      role="img"
      aria-label="Raising a hand: the question in one line, and an answer to a similar question shown first: How do I revalidate on focus in React Native?"
    >
      <p className="land-ui-input">
        Revalidate when my React Native app comes back
      </p>
      <p className="land-ui-similar">
        <Bulb />
        Already answered?
        <b>How do I revalidate on focus in React Native?</b>
      </p>
    </div>
  );
}

/** FR-QUEUE-5 and 9: the banner, with "I'm here". */
function YourTurn() {
  return (
    <div
      className="land-ui land-turn"
      role="img"
      aria-label="The banner: It's your turn. @mia-k is answering your question. With a button: I'm here."
    >
      <RaisedHand />
      <p className="land-turn-text">
        <b>It's your turn</b>
        <span>@mia-k is answering your question.</span>
      </p>
      <span className="btn sm land-turn-here">I'm here</span>
    </div>
  );
}

/** FR-FAQ-1 and 2, FR-HAND-8: the FAQ card, with everyone who asked and the entry's own link. */
function AnsweredOnce() {
  return (
    <div
      className="card land-ui"
      role="img"
      aria-label="The FAQ card: How do I retry only on 5xx errors? The answer in two lines, also asked by @ana-ferreira, and the link to its public page."
    >
      <p className="land-faq-head">
        <Bookmark />
        Added to FAQ
      </p>
      <p className="land-ui-title">How do I retry only on 5xx errors?</p>
      <p className="land-faq-answer">
        Use the onErrorRetry option. It receives the error before each retry, so
        return early when error.status is below 500. For 5xx errors, call
        revalidate after a delay to keep the default back off.
      </p>
      <p className="land-faq-also">Also asked by @ana-ferreira</p>
      <p className="land-faq-foot">
        <span className="land-faq-path">
          /faq/how-do-i-retry-only-on-5xx-errors
        </span>
        <span className="land-faq-open">
          Open answer
          <ArrowUpRight />
        </span>
      </p>
    </div>
  );
}

/** FR-SESS-4 and 7: the card at the end, with the hands carried to the next session. */
function SessionEnded() {
  return (
    <div
      className="card land-ui"
      role="img"
      aria-label="The card at the end: office hours ended. 12 questions, 9 answered, 3 carried to Thursday."
    >
      <p className="land-ui-title">Office hours ended</p>
      <p className="land-ui-stats">
        <span>
          <b>12</b>questions
        </span>
        <span>
          <b>9</b>answered
        </span>
        <span>
          <b>3</b>carried to Thursday
        </span>
      </p>
    </div>
  );
}
