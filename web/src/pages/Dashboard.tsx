import { useEffect, useState } from "react";
import type { FormEvent } from "react";

import {
  claudeCodeCommand,
  disconnectAgent,
  lastUsedLabel,
  listAgents,
  mcpAddress,
} from "../agents";
import type { Agent } from "../agents";
import { errorMessage, refusalOf } from "../api";
import type { Sending } from "../api";
import { retryAuth, signInHref, useAuth } from "../auth";
import { GitHubMark } from "../components/Icons";
import { Link } from "../components/Link";
import { AskOnce } from "../room/panel/AskOnce";
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
        <ConnectedApps />
      </div>
    </div>
  );
}

type Apps =
  | { readonly status: "loading" }
  | {
      readonly status: "ready";
      readonly agents: readonly Agent[];
      /** When the list arrived: "Used 10 min ago" counts from here. */
      readonly at: number;
    }
  | { readonly status: "failed"; readonly message: string };

/**
 * FR-AGENT-1, FR-AGENT-3: where an AI app connects (the MCP address, and the command for Claude
 * Code), then the apps the person let act for them, each with Disconnect, which refuses the app
 * from its next request on.
 */
function ConnectedApps() {
  const [apps, setApps] = useState<Apps>({ status: "loading" });
  const [busyId, setBusyId] = useState<string | null>(null);
  const [refusal, setRefusal] = useState("");
  const [copied, setCopied] = useState("");
  const address = mcpAddress(window.location.origin);
  const command = claudeCodeCommand(address);

  function handleCopy(text: string) {
    navigator.clipboard.writeText(text).then(
      () => setCopied(text),
      () => setCopied(""),
    );
  }

  useEffect(() => {
    let isCurrent = true;
    listAgents().then(
      (agents) => {
        if (isCurrent) setApps({ status: "ready", agents, at: Date.now() });
      },
      (failure: unknown) => {
        if (isCurrent)
          setApps({ status: "failed", message: errorMessage(failure) });
      },
    );
    return () => {
      isCurrent = false;
    };
  }, []);

  async function handleDisconnect(agent: Agent) {
    setBusyId(agent.id);
    setRefusal("");
    try {
      await disconnectAgent(agent.id);
      setApps((current) =>
        current.status === "ready"
          ? {
              ...current,
              agents: current.agents.filter((other) => other.id !== agent.id),
            }
          : current,
      );
    } catch (failure) {
      setRefusal(errorMessage(failure));
    } finally {
      setBusyId(null);
    }
  }

  const agents = apps.status === "ready" ? apps.agents : [];
  return (
    <section className="apps" aria-labelledby="apps-title">
      <h2 id="apps-title">Use Office Hours from your AI app</h2>
      <p className="apps-lead">
        Add this address as an MCP server in Claude, ChatGPT, Cursor or another
        app that speaks MCP. It asks you to sign in here and to allow it, then
        it can do what you can for 30 days, or until you disconnect it.
      </p>
      <div className="mcp-copy">
        <code>{address}</code>
        <button
          type="button"
          className="btn ghost sm"
          onClick={() => handleCopy(address)}
        >
          {copied === address ? "Copied" : "Copy"}
        </button>
      </div>
      <p className="apps-lead">In Claude Code:</p>
      <div className="mcp-copy">
        <code>{command}</code>
        <button
          type="button"
          className="btn ghost sm"
          onClick={() => handleCopy(command)}
        >
          {copied === command ? "Copied" : "Copy"}
        </button>
      </div>
      {apps.status === "failed" && (
        <p className="notice" role="alert">
          <span>Your connected apps couldn't be loaded. {apps.message}</span>
        </p>
      )}
      {agents.length > 0 && apps.status === "ready" && (
        <ConnectedList
          agents={agents}
          at={apps.at}
          busyId={busyId}
          onDisconnect={(agent) => void handleDisconnect(agent)}
        />
      )}
      {refusal && (
        <p className="err" role="alert">
          {refusal}
        </p>
      )}
    </section>
  );
}

type ConnectedListProps = {
  readonly agents: readonly Agent[];
  /** When the list arrived: "Used 10 min ago" counts from here. */
  readonly at: number;
  readonly busyId: string | null;
  readonly onDisconnect: (agent: Agent) => void;
};

/** The apps connected now, newest first, each with Disconnect (asked once). */
function ConnectedList({
  agents,
  at,
  busyId,
  onDisconnect,
}: ConnectedListProps) {
  return (
    <>
      <h3 className="apps-sub">Connected now</h3>
      <ul className="projects">
        {agents.map((agent) => (
          <li key={agent.id} className="app-row">
            <span className="project-name">{agent.app}</span>
            <span className="project-about">
              Returns to {agent.returnsTo} · Connected{" "}
              {new Date(agent.connectedAt).toLocaleDateString(undefined, {
                day: "numeric",
                month: "short",
              })}{" "}
              · {lastUsedLabel(agent.lastUsedAt, at)}
            </span>
            <div className="app-actions">
              <AskOnce
                label="Disconnect"
                question={`Disconnect ${agent.app}? It can't act for you after this.`}
                confirm="Disconnect"
                isBusy={busyId === agent.id}
                onConfirm={() => onDisconnect(agent)}
              />
            </div>
          </li>
        ))}
      </ul>
    </>
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
