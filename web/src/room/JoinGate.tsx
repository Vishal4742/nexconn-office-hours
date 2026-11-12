import { useState } from "react";

import { ApiError, errorMessage } from "../api";
import { signInHref } from "../auth";
import { GitHubMark } from "../components/Icons";
import { joinProject, roomPath } from "../projects";
import type { Project } from "../projects";

type Props = {
  readonly project: Project;
  /** Null while the page does not know yet who is signed in. */
  readonly isSignedIn: boolean | null;
  readonly onJoined: (project: Project) => void;
};

type Joining =
  | { readonly status: "idle" }
  | { readonly status: "sending" }
  | {
      readonly status: "refused";
      readonly message: string;
      readonly needsSignIn: boolean;
    };

/** What someone sees before they are in the room, and the one way in (FR-PROJ-3, FR-PROJ-4). */
export function JoinGate({ project, isSignedIn, onJoined }: Props) {
  const [joining, setJoining] = useState<Joining>({ status: "idle" });
  const signIn = signInHref(roomPath(project));

  async function handleJoin() {
    setJoining({ status: "sending" });
    try {
      onJoined(await joinProject(project));
    } catch (failure) {
      setJoining({
        status: "refused",
        message: errorMessage(failure),
        needsSignIn: failure instanceof ApiError && failure.status === 401,
      });
    }
  }

  return (
    <div className="gate">
      <div className="gate-card">
        <p className="repo muted">
          {project.owner}/{project.repo}
        </p>
        <h1>Join the office hours room</h1>
        {project.description && <p className="muted">{project.description}</p>}
        <p>
          Ask the maintainers a question when they hold office hours, or follow
          along.
        </p>
        {isSignedIn === true && (
          <button
            type="button"
            className="btn primary lg block"
            onClick={handleJoin}
            disabled={joining.status === "sending"}
          >
            {joining.status === "sending" ? "Joining…" : "Join the room"}
          </button>
        )}
        {isSignedIn === false && (
          <a className="btn primary lg block" href={signIn}>
            <GitHubMark />
            Sign in with GitHub to join
          </a>
        )}
        {joining.status === "refused" && (
          <p className="err" role="alert">
            {joining.message}{" "}
            {joining.needsSignIn && <a href={signIn}>Sign in again</a>}
          </p>
        )}
        <p className="fine muted">
          Office Hours sees your public GitHub profile only. No email, no repo
          access.
        </p>
      </div>
    </div>
  );
}
