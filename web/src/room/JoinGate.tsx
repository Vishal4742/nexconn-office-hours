import { useEffect, useState } from "react";

import { refusalOf } from "../api";
import type { Sending } from "../api";
import { signInHref } from "../auth";
import { AddToCalendar } from "../components/AddToCalendar";
import { CalendarIcon, GitHubMark } from "../components/Icons";
import { faqPath, latestFaq } from "../faq";
import type { FaqEntry } from "../faq";
import { joinProject, roomPath } from "../projects";
import type { Project, RepoName } from "../projects";
import { loadQueue } from "../queue/api";
import type { Queue } from "../queue/types";
import { sessionNow } from "../stats/sessionNow";

type Props = {
  readonly project: Project;
  /** Null while the page does not know yet who is signed in. */
  readonly isSignedIn: boolean | null;
  readonly onJoined: (project: Project) => void;
};

/** What the page shows besides the way in. It may fail; joining never waits for it. */
type Extra<T> =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly value: T }
  | { readonly status: "failed" };

/** The server sends 5; more would push the way in out of view. */
const LATEST = 5;

/** What someone sees before they are in the room, and the one way in (FR-PROJ-3, FR-PROJ-4). */
export function JoinGate({ project, isSignedIn, onJoined }: Props) {
  const [joining, setJoining] = useState<Sending>({ status: "idle" });
  const [queue, setQueue] = useState<Extra<Queue>>({ status: "loading" });
  const [answers, setAnswers] = useState<Extra<readonly FaqEntry[]>>({
    status: "loading",
  });
  const { owner, repo } = project;
  const signIn = signInHref(roomPath(project));

  // FR-PROJ-3: whether a session is live and what the room has answered. Both are public, and
  // someone outside the room gets the session without its hands.
  useEffect(() => {
    let isCurrent = true;
    const name = { owner, repo };
    loadQueue(name).then(
      (value) => {
        if (isCurrent) setQueue({ status: "ready", value });
      },
      () => {
        if (isCurrent) setQueue({ status: "failed" });
      },
    );
    latestFaq(name).then(
      (value) => {
        if (isCurrent) setAnswers({ status: "ready", value });
      },
      () => {
        if (isCurrent) setAnswers({ status: "failed" });
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [owner, repo]);

  async function handleJoin() {
    setJoining({ status: "sending" });
    try {
      onJoined(await joinProject(project));
    } catch (failure) {
      setJoining({ status: "refused", ...refusalOf(failure) });
    }
  }

  return (
    <div className="gate">
      <div className="gate-card">
        <p className="repo muted">
          {owner}/{repo}
        </p>
        <h1>Join the office hours room</h1>
        {project.description && <p className="muted">{project.description}</p>}
        <SessionLine queue={queue} repo={project} />
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
        <LatestAnswers repo={project} answers={answers} />
      </div>
    </div>
  );
}

type SessionLineProps = {
  readonly queue: Extra<Queue>;
  readonly repo: RepoName;
};

/** A live session with its title, or the next one in the viewer's time zone (FR-SESS-5). */
function SessionLine({ queue, repo }: SessionLineProps) {
  if (queue.status === "loading") return null;
  if (queue.status === "failed") {
    return <p className="muted">Couldn't check whether a session is live.</p>;
  }
  const now = sessionNow(queue.value);
  if (now.kind === "live") {
    return (
      <p className="gate-live">
        <span className="gate-live-label">
          <span className="gate-dot" aria-hidden="true" />
          Live now
        </span>
        <span className="gate-live-title">{now.title}</span>
      </p>
    );
  }
  if (now.kind === "next") {
    return (
      <>
        <p className="gate-next">
          <CalendarIcon />
          <span>
            Next session: <time dateTime={now.at}>{now.when}</time>, your time
          </span>
        </p>
        <AddToCalendar repo={repo} startsAt={now.at} />
      </>
    );
  }
  return <p className="muted">No session is live or planned right now.</p>;
}

type LatestAnswersProps = {
  readonly repo: RepoName;
  readonly answers: Extra<readonly FaqEntry[]>;
};

/** The room's newest answers. The Go server renders their pages, so these are plain links. */
function LatestAnswers({ repo, answers }: LatestAnswersProps) {
  if (answers.status === "loading") return null;
  if (answers.status === "failed") {
    return (
      <p className="muted">
        The latest answers could not be loaded.{" "}
        <a href={faqPath(repo)}>Browse all answers</a>
      </p>
    );
  }
  if (answers.value.length === 0) return null;
  return (
    <section className="gate-answers" aria-labelledby="gate-answers-h">
      <h2 id="gate-answers-h">Latest answers, no sign-in needed</h2>
      <ul>
        {answers.value.slice(0, LATEST).map((entry) => (
          <li key={entry.slug}>
            <a href={faqPath(repo, entry.slug)}>{entry.title}</a>
          </li>
        ))}
      </ul>
      <a className="gate-more" href={faqPath(repo)}>
        Browse all answers
      </a>
    </section>
  );
}
