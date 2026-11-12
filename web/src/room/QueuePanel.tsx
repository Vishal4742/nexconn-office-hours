import { useState } from "react";

import { Avatar } from "../components/Avatar";
import type { Person } from "../nexconn/roster";
import type { Project } from "../projects";
import { sessionLight } from "../queue/light";
import type { Light } from "../queue/light";
import { queueNews } from "../queue/news";
import { refreshQueue } from "../queue/store";
import type { QueueState } from "../queue/store";
import { elapsedLabel } from "../queue/times";
import type { Hand, Queue, QueueSession } from "../queue/types";
import { LiveQueue } from "./panel/LiveQueue";
import { MainAction } from "./panel/MainAction";
import { OffQueue } from "./panel/OffQueue";
import { useServerNow } from "./panel/useServerNow";

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
/** The same scene at four times of day, in the order the session goes through them. */
const LIGHTS: readonly Light[] = ["day", "golden", "dusk", "night"];
/** How often the session's clock is read again: it shows whole minutes. */
const TICK_MS = 15_000;

/**
 * The photo panel beside the chat (docs/UI.md S3): the live queue floats over the sky as glass,
 * a window onto the desk stays free, and the one thing to do now sits at the bottom. It shows
 * whatever the queue store holds, so it is right whenever the store is (FR-QUEUE-4).
 */
export function QueuePanel({
  project,
  members,
  state,
  onStart,
  onRaise,
  onDone,
}: Props) {
  const queue = state.status === "ready" ? state.queue : null;
  const news = useNews(queue);
  const session = queue?.session ?? null;
  // The light changes every few minutes at most; reading the clock twice a minute is plenty.
  const light = sessionLight(session, useServerNow(30_000));

  return (
    <section
      className="pane-queue"
      aria-label="Queue"
      tabIndex={-1}
      data-light={light}
    >
      {/* ponytail: all four photos load with the room (about 270 KB together), so the next one is
          ready to fade in. Load them as the session reaches each light if phones need the bytes. */}
      {LIGHTS.map((each) => (
        <img
          key={each}
          className={`q-scene q-scene-${each}${each === light ? " is-on" : ""}`}
          src={`/panel-${each}.webp`}
          alt=""
          decoding="async"
        />
      ))}
      {/* NFR-A11Y-1: always in the page, so a screen reader hears each change of the queue. */}
      <p className="sr-only" role="status">
        {news}
      </p>
      <div className="q-top">
        {session?.live && <SessionClock session={session} />}
        <Members members={members} />
      </div>
      <div className="q-body">
        {state.status === "loading" && <Loading />}
        {state.status === "failed" && <Failed message={state.message} />}
        {queue && session?.live && (
          <LiveQueue
            project={project}
            queue={queue}
            session={session}
            isMaintainer={project.isMaintainer}
            onDone={onDone}
          />
        )}
        {queue && !session?.live && (
          <OffQueue
            project={project}
            queue={queue}
            isMaintainer={project.isMaintainer}
          />
        )}
      </div>
      <div className="q-window" aria-hidden="true" />
      {queue && (
        <MainAction
          queue={queue}
          isMaintainer={project.isMaintainer}
          onStart={onStart}
          onRaise={onRaise}
        />
      )}
    </section>
  );
}

/** What the polite line says: what changed since the queue shown before (NFR-A11Y-1). */
function useNews(queue: Queue | null): string {
  const [seen, setSeen] = useState<{
    readonly queue: Queue | null;
    readonly news: string;
  }>({ queue, news: "" });
  if (seen.queue !== queue) {
    setSeen({ queue, news: queue ? queueNews(seen.queue, queue) : "" });
  }
  return seen.news;
}

/** How long the session has run, on the server's clock. */
function SessionClock({ session }: { readonly session: QueueSession }) {
  const now = useServerNow(TICK_MS);
  return (
    <p className="q-chip">
      <span className="q-live-dot" aria-hidden="true" />
      <span>
        {session.handsClosed ? "Last call" : "Live"} ·{" "}
        {elapsedLabel(session.startedAt, now)}
      </span>
    </p>
  );
}

function Members({ members }: { readonly members: readonly Person[] }) {
  if (members.length === 0) return null;
  return (
    <p className="q-chip q-members">
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
      <span>
        {members.length === 1 ? "1 member" : `${members.length} members`}
      </span>
    </p>
  );
}

function Loading() {
  return (
    <div className="q-loading" role="status">
      <span className="sr-only">Loading the queue…</span>
      <div className="q-skel q-skel-now" />
      <div className="q-skel q-skel-list" />
    </div>
  );
}

/** Only when there is nothing to show: a queue that was loaded stays on screen. */
function Failed({ message }: { readonly message: string }) {
  return (
    <section className="qcard q-note" role="alert">
      <p>{message}</p>
      <button type="button" className="btn ghost sm" onClick={refreshQueue}>
        Try again
      </button>
    </section>
  );
}
