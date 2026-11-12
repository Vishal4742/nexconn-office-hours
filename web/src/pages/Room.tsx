import { useEffect, useState } from "react";

import { ApiError, errorMessage } from "../api";
import { useAuth } from "../auth";
import { Link } from "../components/Link";
import { loadProject } from "../projects";
import type { Project, RepoName } from "../projects";
import { ChatRoom } from "../room/ChatRoom";
import { JoinGate } from "../room/JoinGate";

type Page =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly project: Project }
  | { readonly status: "missing" }
  | { readonly status: "failed"; readonly message: string };

/**
 * S3, /p/owner/repo: the project's room for its members, and the way in for everyone else.
 * App gives it a key per repo, so going from one room to another starts from nothing.
 */
export function Room({ owner, repo }: RepoName) {
  const auth = useAuth();
  const [page, setPage] = useState<Page>({ status: "loading" });
  const [loads, setLoads] = useState(0);

  useEffect(() => {
    let isCurrent = true;
    loadProject({ owner, repo }).then(
      (project) => {
        if (isCurrent) setPage({ status: "ready", project });
      },
      (failure: unknown) => {
        if (!isCurrent) return;
        const isMissing = failure instanceof ApiError && failure.status === 404;
        setPage(
          isMissing
            ? { status: "missing" }
            : { status: "failed", message: errorMessage(failure) },
        );
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [owner, repo, loads]);

  function handleRetry() {
    setPage({ status: "loading" });
    setLoads((count) => count + 1);
  }

  if (page.status === "loading") {
    return (
      <div className="wrap page" role="status" aria-label="Loading the room">
        <div className="skel" />
      </div>
    );
  }
  if (page.status === "missing") {
    return (
      <div className="wrap page">
        <h1>
          Nobody hosts office hours for {owner}/{repo} yet
        </h1>
        <p className="lead">
          If you maintain it, you can give it a room in a minute.
        </p>
        <div className="actions">
          <Link href="/dashboard" className="btn primary">
            Host office hours
          </Link>
        </div>
      </div>
    );
  }
  if (page.status === "failed") {
    return (
      <div className="wrap page">
        <h1>
          {owner}/{repo}
        </h1>
        <p className="notice" role="alert">
          <span>{page.message}</span>
          <button type="button" className="btn ghost sm" onClick={handleRetry}>
            Try again
          </button>
        </p>
      </div>
    );
  }

  const { project } = page;
  const isSignedIn =
    auth.status === "loading" ? null : auth.status === "signedIn";
  return (
    <div className="wrap room">
      <header className="room-head">
        <div>
          <h1 className="room-name">
            {project.owner}/{project.repo}
          </h1>
          {project.description && (
            <p className="room-about">{project.description}</p>
          )}
        </div>
        {project.isMember && (
          <span className="tag">
            {project.isMaintainer ? "Maintainer" : "Member"}
          </span>
        )}
      </header>
      {project.groupId ? (
        <ChatRoom
          groupId={project.groupId}
          owner={project.owner}
          repo={project.repo}
        />
      ) : (
        <JoinGate
          project={project}
          isSignedIn={isSignedIn}
          onJoined={(joined) => setPage({ status: "ready", project: joined })}
        />
      )}
    </div>
  );
}
