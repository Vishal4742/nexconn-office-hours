import { Avatar } from "../components/Avatar";
import { CalendarIcon } from "../components/Icons";
import type { Person } from "../nexconn/roster";

type Props = {
  /** Who is in the room, once the chat has loaded them. */
  readonly members: readonly Person[];
};

const FACES = 3;

/**
 * The photo panel beside the chat (docs/UI.md S3). The queue will float over its sky; until a
 * project can hold a session, the panel says that none is running.
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
