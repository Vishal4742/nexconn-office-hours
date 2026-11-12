import type { Me } from "../auth";
import { Avatar } from "../components/Avatar";
import { Logo } from "../components/Icons";
import { Link } from "../components/Link";
import { faqPath } from "../faq";
import { roomPath } from "../projects";
import type { Project } from "../projects";
import type { Queue, QueueSession } from "../queue/types";
import { CopyLink } from "./CopyLink";
import { RoomTabs } from "./head/RoomTabs";
import { SessionControls } from "./head/SessionControls";
import { LIVE_LABEL, headline, liveState, metaLine } from "./head/text";
import { useServerNow } from "./panel/useServerNow";

/** Below 1040 px the room shows one pane at a time (docs/UI.md, "S3 on phones"). */
export type RoomTab = "queue" | "chat";

type Props = {
  readonly project: Project;
  readonly me: Me | null;
  /** Null until the queue has loaded. */
  readonly queue: Queue | null;
  readonly tab: RoomTab;
  readonly onTab: (tab: RoomTab) => void;
  /** Maintainer, no live session: opens the start sheet. */
  readonly onStart: () => void;
  /** Maintainer, live session: opens the end sheet (ending asks first). */
  readonly onEnd: () => void;
};

/** The minutes in the head move on twice a minute. */
const TICK_MS = 30_000;

function LiveLabel({ session }: { readonly session: QueueSession | null }) {
  const state = liveState(session);
  if (state === "off") return <span className="off">{LIVE_LABEL.off}</span>;
  return (
    <span className="rh-live" data-state={state}>
      <span className="rh-dot" aria-hidden="true" />
      {LIVE_LABEL[state]}
    </span>
  );
}

/** The session as a headline, its state the last word in blue italic serif. */
function Title({ queue }: { readonly queue: Queue | null }) {
  if (!queue) {
    return (
      <h1 className="rh-title" aria-busy="true">
        <span className="sr-only">Loading the session…</span>
        <span className="rh-skel" aria-hidden="true" />
      </h1>
    );
  }
  const { name, state } = headline(queue.session);
  // The title gives way to an ellipsis, never the state: the label names it whole.
  return (
    <h1 className="rh-title" aria-label={`${name} ${state}`}>
      <span className="rh-name">{name}</span>
      <span className="serif">{state}</span>
    </h1>
  );
}

/**
 * The room's top (docs/UI.md S3): the brand row with the live label and the room's links, the
 * session as a headline with one line of its real state, a maintainer's session controls, and
 * below 1040 px the tabs that choose the pane. The props are a contract (notes/phase-2-plan.md).
 */
export function RoomHead({
  project,
  me,
  queue,
  tab,
  onTab,
  onStart,
  onEnd,
}: Props) {
  const now = useServerNow(TICK_MS); // the server's clock (plan decision 12)
  const session = queue?.session ?? null;
  const meta = metaLine(session, now);
  const hasControls = project.isMaintainer && queue !== null;
  const waiting =
    queue?.hands.filter((hand) => hand.status === "waiting").length ?? 0;
  const whoIAm = me
    ? `${me.name}, ${project.isMaintainer ? "maintainer" : "member"}`
    : "";

  return (
    <header className="room-head">
      <div className="rh-brand">
        <Link href="/" className="brand" aria-label="Office Hours home">
          <Logo />
          <span className="brand-name">Office Hours</span>
        </Link>
        <span className="slash brand-name" aria-hidden="true">
          /
        </span>
        <span className="repo" title={`${project.owner}/${project.repo}`}>
          {project.owner}/{project.repo}
        </span>
        {queue && <LiveLabel session={session} />}
        <span className="grow" />
        <div className="rh-controls">
          <CopyLink path={roomPath(project)} />
          {/* The Go server renders the FAQ pages: a plain link, not the app's. */}
          <a href={faqPath(project)} className="text-btn">
            FAQ
          </a>
          <Link href={`${roomPath(project)}/stats`} className="text-btn">
            Numbers
          </Link>
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
      <Title queue={queue} />
      {(meta || hasControls) && (
        <div className="rh-session">
          {meta && <p className="rh-meta">{meta}</p>}
          {hasControls && (
            <SessionControls
              key={session?.id ?? "none"}
              session={session}
              onStart={onStart}
              onEnd={onEnd}
            />
          )}
        </div>
      )}
      <RoomTabs tab={tab} onTab={onTab} waiting={waiting} />
    </header>
  );
}
