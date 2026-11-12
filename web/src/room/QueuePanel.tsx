import { Avatar } from "../components/Avatar";
import { CalendarIcon } from "../components/Icons";
import type { Person } from "../nexconn/roster";
import type { Project } from "../projects";
import type { QueueState } from "../queue/store";
import type { Hand } from "../queue/types";

type Props = {
  readonly project: Project;
  /** Who is in the room, once the chat has loaded them. */
  readonly members: readonly Person[];
  readonly state: QueueState;
  /** Maintainer, no live session: opens the start sheet. */
  readonly onStart: () => void;
  /** Opens the raise-hand sheet. */
  readonly onRaise: () => void;
  /** Maintainer: opens the Done sheet for the hand being answered. */
  readonly onDone: (hand: Hand) => void;
};

const FACES = 3;

/**
 * The photo panel beside the chat (docs/UI.md S3). The props are a contract
 * (notes/phase-2-plan.md); the queue that floats over the photo's sky is the queue-panel part of
 * the build (worker 5). Until then the panel says that no session is running.
 */
export function QueuePanel({ members }: Props) {
  const count = members.length === 1 ? "1 member" : `${members.length} members`;
  return (
    <section className="pane-queue" aria-label="Queue">
      <img className="q-scene" src="/panel-day.webp" alt="" />
      <div className="q-top">
        {members.length > 0 && (
          <p className="q-chip">
            <span className="av-stack">
              {members.slice(0, FACES).map((person) => (
                <Avatar
                  key={person.userId}
                  name={person.name}
                  url={person.avatarUrl}
                  size="sm"
                />
              ))}
            </span>
            <span>{count}</span>
          </p>
        )}
      </div>
      <div className="qcard off-card">
        <CalendarIcon />
        <div>
          <p className="off-title">No session right now</p>
          <p className="muted">
            When a session is live, the queue of questions shows here.
          </p>
        </div>
      </div>
    </section>
  );
}
