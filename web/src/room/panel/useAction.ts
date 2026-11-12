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
 * One control's calls to the server: busy while a call is on its way, then what the server
 * refused, or the sentence given for a success. A call that changes the queue gives the server's
 * answer to acceptQueue itself.
 */
export function useAction() {
  const [doing, setDoing] = useState<Doing>({ status: "idle" });

  async function run(call: () => Promise<void>, said = ""): Promise<void> {
    setDoing({ status: "sending" });
    try {
      await call();
      setDoing(said ? { status: "done", message: said } : { status: "idle" });
    } catch (failure) {
      setDoing({ status: "refused", ...refusalOf(failure) });
    }
  }

  return { doing, run, isBusy: doing.status === "sending" };
}
