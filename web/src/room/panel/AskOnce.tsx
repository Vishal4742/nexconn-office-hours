import { useState } from "react";

type Props = {
  /** The button that asks. */
  readonly label: string;
  readonly question: string;
  /** The button that does it. */
  readonly confirm: string;
  readonly isBusy: boolean;
  readonly onConfirm: () => void;
  readonly className?: string;
};

/**
 * A button that asks once, inline, before it does something that can't be taken back (Remove,
 * Withdraw, Remove from the FAQ). Focus moves to the question and back, so a keyboard is never
 * left on a button that is gone.
 */
export function AskOnce({
  label,
  question,
  confirm,
  isBusy,
  onConfirm,
  className = "btn ghost sm",
}: Props) {
  const [isAsking, setIsAsking] = useState(false);
  const [wasAsked, setWasAsked] = useState(false);

  if (!isAsking) {
    return (
      <button
        type="button"
        className={className}
        disabled={isBusy}
        autoFocus={wasAsked}
        onClick={() => setIsAsking(true)}
      >
        {label}
      </button>
    );
  }
  return (
    <span className="q-ask" role="group" aria-label={question}>
      <span>{question}</span>
      <button
        type="button"
        className="btn primary sm"
        disabled={isBusy}
        autoFocus
        onClick={onConfirm}
      >
        {confirm}
      </button>
      <button
        type="button"
        className="btn ghost sm"
        onClick={() => {
          setIsAsking(false);
          setWasAsked(true);
        }}
      >
        Keep it
      </button>
    </span>
  );
}
