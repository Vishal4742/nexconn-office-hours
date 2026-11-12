import { useEffect, useState } from "react";

import { ApiError, errorMessage } from "../api";
import { useAuth } from "../auth";
import { Header } from "../components/Header";
import { Link } from "../components/Link";
import type { Person } from "../nexconn/roster";
import { loadProject } from "../projects";
import type { Project, RepoName } from "../projects";
import { useQueue, watchQueue } from "../queue/store";
import type { Hand } from "../queue/types";
import { ChatRoom } from "../room/ChatRoom";
import { JoinGate } from "../room/JoinGate";
import { QueuePanel } from "../room/QueuePanel";
import { RoomHead } from "../room/RoomHead";
import type { RoomTab } from "../room/RoomHead";
import { DoneSheet } from "../room/sheets/DoneSheet";
import { EndSessionSheet } from "../room/sheets/EndSessionSheet";
import { RaiseHandSheet } from "../room/sheets/RaiseHandSheet";
import { StartSessionSheet } from "../room/sheets/StartSessionSheet";
import { YourTurnBanner } from "../room/YourTurnBanner";

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
        <div className="wrap page" role="status">
          <span className="sr-only">Loading the room…</span>
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

/** The one sheet that is open over the room, if any. */
type OpenSheet =
  | { readonly kind: "none" }
  | { readonly kind: "start" }
  /** End and raise belong to one session: they never open over the next one. */
  | { readonly kind: "end"; readonly sessionId: string }
  | { readonly kind: "raise"; readonly sessionId: string }
  | { readonly kind: "done"; readonly hand: Hand };

/**
 * The room as a member sees it: the session as a headline with the chat under it, and the photo
 * panel with the queue beside them. This function only puts the parts together and holds what
 * they share (the queue, which sheet is open, which tab shows on a phone). It is a contract of
 * the parallel build (notes/phase-2-plan.md): each part lives in its own file with fixed props.
 */
function MemberRoom({ project, groupId }: MemberRoomProps) {
  const auth = useAuth();
  const queueState = useQueue();
  const [members, setMembers] = useState<readonly Person[]>([]);
  const [sheet, setSheet] = useState<OpenSheet>({ kind: "none" });
  const [tab, setTab] = useState<RoomTab>("queue");
  const { owner, repo, isMaintainer } = project;

  useEffect(
    () => watchQueue({ owner, repo, groupId, isMaintainer }),
    [owner, repo, groupId, isMaintainer],
  );

  const me = auth.status === "signedIn" ? auth.me : null;
  const queue = queueState.status === "ready" ? queueState.queue : null;
  const session = queue?.session ?? null;
  const waitingCount =
    queue?.hands.filter((hand) => hand.status === "waiting").length ?? 0;
  const closeSheet = () => setSheet({ kind: "none" });

  return (
    <div className="room" data-tab={tab}>
      <RoomHead
        project={project}
        me={me}
        queue={queue}
        tab={tab}
        onTab={setTab}
        onStart={() => setSheet({ kind: "start" })}
        onEnd={() =>
          session && setSheet({ kind: "end", sessionId: session.id })
        }
      />
      <YourTurnBanner queue={queue} />
      <ChatRoom
        groupId={groupId}
        owner={owner}
        repo={repo}
        onMembers={setMembers}
      />
      <QueuePanel
        project={project}
        members={members}
        state={queueState}
        onStart={() => setSheet({ kind: "start" })}
        onRaise={() =>
          session && setSheet({ kind: "raise", sessionId: session.id })
        }
        onDone={(hand) => setSheet({ kind: "done", hand })}
      />
      {sheet.kind === "start" && (
        <StartSessionSheet project={project} onClose={closeSheet} />
      )}
      {sheet.kind === "end" &&
        session?.live &&
        sheet.sessionId === session.id && (
          <EndSessionSheet
            session={session}
            waitingCount={waitingCount}
            onClose={closeSheet}
          />
        )}
      {sheet.kind === "raise" &&
        session?.live &&
        sheet.sessionId === session.id && (
          <RaiseHandSheet
            project={project}
            session={session}
            onClose={closeSheet}
          />
        )}
      {sheet.kind === "done" && (
        <DoneSheet project={project} hand={sheet.hand} onClose={closeSheet} />
      )}
    </div>
  );
}
