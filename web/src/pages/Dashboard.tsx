import { useEffect, useState } from "react";
import type { FormEvent } from "react";

import { errorMessage, refusalOf } from "../api";
import type { Sending } from "../api";
import { retryAuth, signInHref, useAuth } from "../auth";
import { GitHubMark } from "../components/Icons";
import { Link } from "../components/Link";
import {
  isSameRepo,
  listMyProjects,
  parseRepo,
  registerProject,
  roomPath,
} from "../projects";
import type { Project } from "../projects";

/** S2: the rooms I am in, and the form that gives a repo I maintain a room of its own. */
export function Dashboard() {
  const auth = useAuth();

  if (auth.status === "loading") {
    return (
      <div className="wrap page" aria-busy="true">
        <h1>Your projects</h1>
        <p className="empty">Loading…</p>
      </div>
    );
  }
  if (auth.status === "unreachable") {
    return (
      <div className="wrap page">
        <h1>Your projects</h1>
        <p className="empty" role="alert">
          {auth.message}
        </p>
        <div className="actions">
          <button type="button" className="btn ghost" onClick={retryAuth}>
            Try again
          </button>
        </div>
      </div>
    );
  }
  if (auth.status === "signedOut") {
    return (
      <div className="wrap page">
        <h1>Your projects</h1>
        <p className="lead">
          Sign in with GitHub to host office hours for a repo you maintain, or
          to see the rooms you joined.
        </p>
        <div className="actions">
          <a className="btn primary lg" href={signInHref("/dashboard")}>
            <GitHubMark />
            Sign in with GitHub
          </a>
        </div>
      </div>
    );
  }
  return <Projects />;
}

type List =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly projects: readonly Project[] }
  | { readonly status: "failed"; readonly message: string };

/** What the last registration did, said once above the list. */
type Added = { readonly project: Project; readonly wasListed: boolean };

function Projects() {
  const [list, setList] = useState<List>({ status: "loading" });
  const [loads, setLoads] = useState(0);
  const [added, setAdded] = useState<Added | null>(null);

  useEffect(() => {
    let isCurrent = true;
    listMyProjects().then(
      (projects) => {
        if (isCurrent) setList({ status: "ready", projects });
      },
      (failure: unknown) => {
        if (isCurrent)
          setList({ status: "failed", message: errorMessage(failure) });
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [loads]);

  function handleRetry() {
    setList({ status: "loading" });
    setLoads((count) => count + 1);
  }

  function handleRegistered(project: Project) {
    const wasListed =
      list.status === "ready" &&
      list.projects.some((listed) => isSameRepo(listed, project));
    setAdded({ project, wasListed });
    // The server owns the order of the list, so the list is asked for again. What is on screen
    // stays there until the new one arrives.
    setLoads((count) => count + 1);
  }

  return (
    <div className="wrap page">
      <h1>Your projects</h1>
      <p className="lead">
        Host office hours for a repo you maintain, or open a room you joined.
      </p>
      <div className="dash">
        <RegisterForm onRegistered={handleRegistered} />
        {/* Always in the page, so a screen reader hears the notice when it appears. */}
        <div role="status">
          {added && (
            <p className="notice">
              <span>
                {added.wasListed
                  ? `${added.project.owner}/${added.project.repo} already has a room, and it is in your list.`
                  : `${added.project.owner}/${added.project.repo} has a room now.`}
              </span>
              <Link href={roomPath(added.project)} className="btn ghost sm">
                Open the room
              </Link>
            </p>
          )}
        </div>
        <ProjectList list={list} onRetry={handleRetry} />
      </div>
    </div>
  );
}

type RegisterFormProps = {
  readonly onRegistered: (project: Project) => void;
};

/** FR-PROJ-1: one field. The server asks GitHub whether the person maintains the repo. */
function RegisterForm({ onRegistered }: RegisterFormProps) {
  const [value, setValue] = useState("");
  const [submission, setSubmission] = useState<Sending>({ status: "idle" });
  const isSending = submission.status === "sending";
  const isRefused = submission.status === "refused";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = parseRepo(value);
    if (!name) {
      setSubmission({
        status: "refused",
        message:
          "Write the repo as owner/repo, like paperplane/stash, or paste its GitHub address.",
        needsSignIn: false,
      });
      return;
    }
    setSubmission({ status: "sending" });
    try {
      const project = await registerProject(name);
      setValue("");
      setSubmission({ status: "idle" });
      onRegistered(project);
    } catch (failure) {
      setSubmission({ status: "refused", ...refusalOf(failure) });
    }
  }

  return (
    <form
      className="card host"
      onSubmit={handleSubmit}
      aria-label="Host office hours for a repo"
      noValidate
    >
      <div className="field">
        <label htmlFor="host-repo">Repository</label>
        <div className="host-row">
          <input
            id="host-repo"
            name="repo"
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder="owner/repo"
            autoComplete="off"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            aria-describedby={isRefused ? "host-hint host-error" : "host-hint"}
            aria-invalid={isRefused}
          />
          <button type="submit" className="btn primary" disabled={isSending}>
            {isSending ? "Asking GitHub…" : "Host office hours"}
          </button>
        </div>
        <p className="hint" id="host-hint">
          A public repo where you have admin or maintain rights. You can paste
          its GitHub address.
        </p>
        {isRefused && (
          <p className="err" role="alert" id="host-error">
            {submission.message}{" "}
            {submission.needsSignIn && (
              <a href={signInHref("/dashboard")}>Sign in again</a>
            )}
          </p>
        )}
      </div>
    </form>
  );
}

type ProjectListProps = {
  readonly list: List;
  readonly onRetry: () => void;
};

function ProjectList({ list, onRetry }: ProjectListProps) {
  if (list.status === "loading") {
    return (
      <div className="projects" role="status">
        <span className="sr-only">Loading your projects…</span>
        <div className="skel" />
        <div className="skel" />
      </div>
    );
  }
  if (list.status === "failed") {
    return (
      <p className="notice" role="alert">
        <span>{list.message}</span>
        <button type="button" className="btn ghost sm" onClick={onRetry}>
          Try again
        </button>
      </p>
    );
  }
  if (list.projects.length === 0) {
    return (
      <p className="empty">
        No projects yet. Register a repo above, or open a room link someone sent
        you.
      </p>
    );
  }
  return (
    <ul className="projects" aria-label="Your projects">
      {list.projects.map((project) => (
        <li key={roomPath(project)}>
          <Link href={roomPath(project)} className="project">
            <span className="project-name">
              {project.owner}/{project.repo}
            </span>
            <span className="tag">
              {project.isMaintainer ? "Maintainer" : "Member"}
            </span>
            {project.description && (
              <span className="project-about">{project.description}</span>
            )}
          </Link>
        </li>
      ))}
    </ul>
  );
}
