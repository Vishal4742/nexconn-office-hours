// What every sheet of the room is made of, and the room's head and banner too: a field with its
// counter, the server's refusal said inline, and one request that answers with the queue.
import { useEffect, useRef, useState } from "react";
import type { ChangeEvent } from "react";

import { refusalOf } from "../../api";
import type { Sending } from "../../api";
import { signInHref } from "../../auth";
import { actOnQueue } from "../../queue/store";
import type { Queue } from "../../queue/types";
import { amount } from "./limits";

type FieldProps = {
  readonly id: string;
  readonly label: string;
  readonly isOptional?: boolean;
  readonly value: string;
  readonly onChange: (value: string) => void;
  /** "count / max" beside the label, in characters as the server counts them. */
  readonly counter?: { readonly count: number; readonly max: number };
  /** What is wrong, said under the field; null when nothing is (or not yet). */
  readonly problem: string | null;
  readonly hint?: string;
  readonly placeholder?: string;
  /** A textarea of this many rows; a one-line input without it. */
  readonly rows?: number;
};

/** One labelled field. Its counter, hint and problem are read with it (aria-describedby). */
export function TextField({
  id,
  label,
  isOptional = false,
  value,
  onChange,
  counter,
  problem,
  hint,
  placeholder,
  rows,
}: FieldProps) {
  const counterId = `${id}-count`;
  const hintId = `${id}-hint`;
  const problemId = `${id}-problem`;
  const isOver = counter !== undefined && counter.count > counter.max;
  const control = {
    id,
    value,
    placeholder,
    "aria-invalid": problem !== null,
    "aria-describedby":
      [counter && counterId, hint && hintId, problem && problemId]
        .filter(Boolean)
        .join(" ") || undefined,
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      onChange(event.target.value),
  };

  return (
    <div className="field">
      <div className="sheet-field-top">
        <label htmlFor={id}>
          {label}
          {isOptional && <span className="sheet-optional"> (optional)</span>}
        </label>
        {counter && (
          <span
            id={counterId}
            className={isOver ? "sheet-count sheet-over" : "sheet-count"}
          >
            {amount(counter.count)} / {amount(counter.max)}
          </span>
        )}
      </div>
      {rows ? (
        <textarea rows={rows} {...control} />
      ) : (
        <input autoComplete="off" {...control} />
      )}
      {hint && (
        <p className="hint" id={hintId}>
          {hint}
        </p>
      )}
      {problem && (
        <p className="err" id={problemId}>
          {problem}
        </p>
      )}
    </div>
  );
}

/** The server's refusal, said where the action was; with a way back in when the sign-in ran out. */
export function Refused({ sending }: { readonly sending: Sending }) {
  if (sending.status !== "refused") return null;
  return (
    <p className="err" role="alert">
      {sending.message}{" "}
      {sending.needsSignIn && (
        <a href={signInHref(window.location.pathname)}>Sign in again</a>
      )}
    </p>
  );
}

type Send = (request: () => Promise<Queue>) => Promise<boolean>;

/**
 * Sends one request that answers with the queue, through actOnQueue, so the room is current
 * before the ping arrives (and an answer about a room the page has left is dropped). `send` is
 * true when it worked and the part that sent it is still there: a sheet closes itself then, and
 * one the person already closed must not close another.
 */
export function useQueueSend(): readonly [Sending, Send] {
  const [sending, setSending] = useState<Sending>({ status: "idle" });
  const isMounted = useRef(false);

  useEffect(() => {
    isMounted.current = true;
    return () => {
      isMounted.current = false;
    };
  }, []);

  async function send(request: () => Promise<Queue>): Promise<boolean> {
    setSending({ status: "sending" });
    try {
      await actOnQueue(request);
    } catch (failure) {
      setSending({ status: "refused", ...refusalOf(failure) });
      return false;
    }
    setSending({ status: "idle" });
    return isMounted.current;
  }

  return [sending, send];
}
