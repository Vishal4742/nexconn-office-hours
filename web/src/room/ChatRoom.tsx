import { useEffect, useRef, useState } from "react";

import { errorMessage } from "../api";
import { loadRoom } from "../nexconn/lazy";
import type { Person } from "../nexconn/roster";
import { listMembers } from "../projects";

type Props = {
  readonly groupId: string;
  readonly owner: string;
  readonly repo: string;
  /** Called with the room's people each time the chat has loaded them. */
  readonly onMembers: (members: readonly Person[]) => void;
};

type Opening =
  | { readonly status: "opening" }
  | { readonly status: "open" }
  | { readonly status: "failed"; readonly message: string };

/** The room's chat (FR-PROJ-4): Chat UI's message list and input for the project's group. */
export function ChatRoom({ groupId, owner, repo, onMembers }: Props) {
  const hostRef = useRef<HTMLDivElement>(null);
  const [opening, setOpening] = useState<Opening>({ status: "opening" });
  const [tries, setTries] = useState(0);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const leaving = new AbortController();
    let close = () => {};
    loadRoom()
      .catch(() => {
        // The browser's own text for this names a script file. Ours says what to do.
        throw new Error(
          "The chat could not be loaded. Check your connection, or reload the page.",
        );
      })
      .then((room) =>
        room.openRoom(
          host,
          {
            groupId,
            name: `${owner}/${repo}`,
            loadMembers: async () => {
              const members = await listMembers({ owner, repo });
              onMembers(members);
              return members;
            },
          },
          leaving.signal,
        ),
      )
      .then(
        (closeRoom) => {
          if (leaving.signal.aborted) {
            closeRoom();
            return;
          }
          close = closeRoom;
          setOpening({ status: "open" });
        },
        (failure: unknown) => {
          if (!leaving.signal.aborted) {
            setOpening({ status: "failed", message: errorMessage(failure) });
          }
        },
      );
    return () => {
      leaving.abort();
      close();
    };
  }, [groupId, owner, repo, onMembers, tries]);

  function handleRetry() {
    setOpening({ status: "opening" });
    setTries((count) => count + 1);
  }

  return (
    <section className="chat" aria-label="Chat">
      {/* Always in the page, so a screen reader hears it; the visible line is below. */}
      <span className="sr-only" role="status">
        {opening.status === "opening" ? "Opening the room…" : ""}
      </span>
      {opening.status === "opening" && (
        <p className="chat-note" aria-hidden="true">
          Opening the room…
        </p>
      )}
      {opening.status === "failed" && (
        <div className="chat-note" role="alert">
          <p>{opening.message}</p>
          <button type="button" className="btn ghost" onClick={handleRetry}>
            Try again
          </button>
        </div>
      )}
      {/* Chat UI's elements are put in here by nexconn/room.ts, not by React. */}
      <div className="chat-host" ref={hostRef} />
    </section>
  );
}
