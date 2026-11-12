import { useState } from "react";

import { refusalOf } from "../../api";
import type { Refusal } from "../../api";

/** What one control of the panel is doing. */
export type Doing =
  | { readonly status: "idle" }
  | { readonly status: "sending" }
  | { readonly status: "done"; readonly message: string }
  | ({ readonly status: "refused" } & Refusal);

/**
 * After an action the control that was pressed may be gone (a row skipped, a hand withdrawn). Then
 * the focus would fall to the page's body and a keyboard user would start again from the top, so
 * the panel takes it (NFR-A11Y-1).
 */
function keepFocusInPanel(): void {
  requestAnimationFrame(() => {
    if (document.activeElement === document.body) {
      document.querySelector<HTMLElement>(".pane-queue")?.focus();
    }
  });
}

/**
 * One control's calls to the server: busy while a call is on its way, then what the server
 * refused, or the sentence given for a success. A call that changes the queue goes through
 * actOnQueue (queue/store.ts), which hands the server's answer to the store.
 */
export function useAction() {
  const [doing, setDoing] = useState<Doing>({ status: "idle" });

  async function run(call: () => Promise<void>, said = ""): Promise<void> {
    setDoing({ status: "sending" });
    try {
      await call();
      setDoing(said ? { status: "done", message: said } : { status: "idle" });
      keepFocusInPanel();
    } catch (failure) {
      setDoing({ status: "refused", ...refusalOf(failure) });
    }
  }

  return { doing, run, isBusy: doing.status === "sending" };
}
