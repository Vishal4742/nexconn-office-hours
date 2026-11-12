import { signInHref } from "../../auth";
import type { Doing } from "./useAction";

type Props = {
  readonly doing: Doing;
};

/** What an action led to, next to the control that was pressed: a refusal is said at once. */
export function Said({ doing }: Props) {
  if (doing.status === "refused") {
    return (
      <p className="q-said q-said-no" role="alert">
        {doing.message}{" "}
        {doing.needsSignIn && (
          <a href={signInHref(window.location.pathname)}>Sign in again</a>
        )}
      </p>
    );
  }
  if (doing.status === "done") {
    return (
      <p className="q-said" role="status">
        {doing.message}
      </p>
    );
  }
  return null;
}
