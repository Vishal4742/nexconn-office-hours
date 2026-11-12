import type { Me } from "../auth";
import { Avatar } from "../components/Avatar";
import { Logo } from "../components/Icons";
import { Link } from "../components/Link";
import { roomPath } from "../projects";
import type { Project } from "../projects";
import type { Queue } from "../queue/types";
import { CopyLink } from "./CopyLink";

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

/**
 * The room's top: the brand row and the session as a headline (docs/UI.md S3). The props are a
 * contract (notes/phase-2-plan.md); the session's state, the maintainer's controls and the tabs
 * are the room-head part of the build (worker 6). Until then it says what Phase 1 said.
 */
export function RoomHead({ project, me }: Props) {
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
  );
}
