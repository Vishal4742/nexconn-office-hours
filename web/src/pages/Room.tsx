import { useEffect, useState } from "react";

import { ApiError, errorMessage } from "../api";
import { useAuth } from "../auth";
import { Avatar } from "../components/Avatar";
import { Header } from "../components/Header";
import { Logo } from "../components/Icons";
import { Link } from "../components/Link";
import type { Person } from "../nexconn/roster";
import { loadProject, roomPath } from "../projects";
import type { Project, RepoName } from "../projects";
import { ChatRoom } from "../room/ChatRoom";
import { CopyLink } from "../room/CopyLink";
import { JoinGate } from "../room/JoinGate";
import { QueuePanel } from "../room/QueuePanel";

type Page =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly project: Project }
  | { readonly status: "missing" }
  | { readonly status: "failed"; readonly message: string };

/**
 * S3, /p/owner/repo: the project's room for its members, and the way in for everyone else.
 * App gives it a key per repo, so going from one room to another starts from nothing. App also
 * leaves the site's header out on this route: the room has a top row of its own, and every other
 * state of this page puts the header back.
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

  useEffect(() => {
    const before = document.title;
    document.title = `${owner}/${repo} · Office Hours`;
    return () => {
      document.title = before;
    };
  }, [owner, repo]);

  function handleRetry() {
    setPage({ status: "loading" });
    setLoads((count) => count + 1);
  }

  if (page.status === "ready" && page.project.groupId) {
    return <MemberRoom project={page.project} groupId={page.project.groupId} />;
  }
  if (page.status === "ready") {
    return (
      <>
        <Header />
        <JoinGate
          project={page.project}
          isSignedIn={
            auth.status === "loading" ? null : auth.status === "signedIn"
          }
          onJoined={(joined) => setPage({ status: "ready", project: joined })}
        />
      </>
    );
  }
  return (
    <>
      <Header />
      {page.status === "loading" && (
        <div className="wrap page" role="status" aria-label="Loading the room">
          <div className="skel" />
        </div>
      )}
      {page.status === "missing" && (
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
      )}
      {page.status === "failed" && (
        <div className="wrap page">
          <h1>
            {owner}/{repo}
          </h1>
          <p className="notice" role="alert">
            <span>{page.message}</span>
            <button
              type="button"
              className="btn ghost sm"
              onClick={handleRetry}
            >
              Try again
            </button>
          </p>
        </div>
      )}
    </>
  );
}

type MemberRoomProps = {
  readonly project: Project;
  readonly groupId: string;
};

/**
 * The room as a member sees it: the session as a headline with the chat under it, and the photo
 * panel beside them. There are no sessions yet, so the headline says so.
 */
function MemberRoom({ project, groupId }: MemberRoomProps) {
  const auth = useAuth();
  const [members, setMembers] = useState<readonly Person[]>([]);
  const me = auth.status === "signedIn" ? auth.me : null;
  const whoIAm = me
    ? `${me.name}, ${project.isMaintainer ? "maintainer" : "member"}`
    : "";

  return (
    <div className="room">
      <header className="room-head">
        <div className="rh-brand">
          <Link href="/" className="brand" aria-label="Office Hours home">
            <Logo />
            <span className="brand-name">Office Hours</span>
          </Link>
          <span className="slash brand-name" aria-hidden="true">
            /
          </span>
          <span className="repo">
            {project.owner}/{project.repo}
          </span>
          <span className="off">Not live</span>
          <span className="grow" />
          <div className="rh-controls">
            <CopyLink path={roomPath(project)} />
            <Link href="/dashboard" className="text-btn wide-only">
              Dashboard
            </Link>
            {me && (
              <span className="rh-me" title={whoIAm}>
                <Avatar name={me.name} url={me.avatarUrl} />
                <span className="sr-only">{whoIAm}</span>
              </span>
            )}
          </div>
        </div>
        <h1 className="rh-title">
          No session <span className="serif">right now.</span>
        </h1>
      </header>
      <ChatRoom
        groupId={groupId}
        owner={project.owner}
        repo={project.repo}
        onMembers={setMembers}
      />
      <QueuePanel members={members} />
    </div>
  );
}
