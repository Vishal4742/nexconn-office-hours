import { useState } from "react";

import type { Me } from "../auth";
import type { Person } from "../nexconn/roster";
import type { Project } from "../projects";
import { ana, diego, liwen, mia, queues } from "../queue/fixtures";
import type { QueueState } from "../queue/store";
import type { Hand, QueuePerson } from "../queue/types";
import { QueuePanel } from "../room/QueuePanel";
import { RoomHead } from "../room/RoomHead";
import type { RoomTab } from "../room/RoomHead";
import { DoneSheet } from "../room/sheets/DoneSheet";
import { EndSessionSheet } from "../room/sheets/EndSessionSheet";
import { RaiseHandSheet } from "../room/sheets/RaiseHandSheet";
import { StartSessionSheet } from "../room/sheets/StartSessionSheet";
import { YourTurnBanner } from "../room/YourTurnBanner";

type FixtureName = keyof typeof queues;
type StateName = "loading" | "failed" | FixtureName;
type Sheet = "none" | "start" | "end" | "raise" | "done";

const STATES: readonly StateName[] = [
  "loading",
  "failed",
  ...(Object.keys(queues) as FixtureName[]),
];
const SHEETS: readonly Sheet[] = ["start", "end", "raise", "done"];

function stateOf(name: StateName): QueueState {
  if (name === "loading") return { status: "loading" };
  if (name === "failed") {
    return {
      status: "failed",
      message: "Can't reach the server. Check your connection and try again.",
    };
  }
  return { status: "ready", queue: queues[name] };
}

const asMe = (person: QueuePerson): Me => ({
  id: person.userId,
  login: person.login,
  name: person.name,
  avatarUrl: person.avatarUrl,
});

const MEMBERS: readonly Person[] = [mia, diego, ana, liwen].map((person) => ({
  userId: person.userId,
  name: person.name,
  avatarUrl: person.avatarUrl,
}));

const BAR = {
  display: "flex",
  flexWrap: "wrap",
  gap: 12,
  alignItems: "center",
  padding: "6px 12px",
  font: "12px/16px ui-monospace, monospace",
  borderBottom: "1px solid #c8c4b8",
} as const;

/**
 * Development only (/dev/room, never in a production build): the room's parts in every state of
 * the queue, from web/src/queue/fixtures.ts, as a member (Diego) or as the maintainer (Mia). No
 * session, no chat and no sign-in are needed to look at a state; the buttons call the real API,
 * so they are refused here.
 */
export function DevRoom() {
  const [name, setName] = useState<StateName>("liveWaiting");
  const [isMaintainer, setIsMaintainer] = useState(false);
  const [tab, setTab] = useState<RoomTab>("queue");
  const [sheet, setSheet] = useState<Sheet>("none");

  const state = stateOf(name);
  const queue = state.status === "ready" ? state.queue : null;
  const project: Project = {
    owner: "PaperPlane",
    repo: "Stash",
    description: "A tiny HTTP cache.",
    isMember: true,
    isMaintainer,
    groupId: "pdevpreview",
  };
  const session = queue?.session ?? queues.liveWatching.session;
  const hand: Hand = queue?.hands[0] ?? queues.liveWatching.hands[0];
  const closeSheet = () => setSheet("none");

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={BAR}>
        <strong>Preview</strong>
        <label>
          state{" "}
          <select
            value={name}
            onChange={(event) => setName(event.target.value as StateName)}
          >
            {STATES.map((option) => (
              <option key={option}>{option}</option>
            ))}
          </select>
        </label>
        <label>
          <input
            type="checkbox"
            checked={isMaintainer}
            onChange={(event) => setIsMaintainer(event.target.checked)}
          />{" "}
          maintainer
        </label>
        {SHEETS.map((option) => (
          <button key={option} type="button" onClick={() => setSheet(option)}>
            {option} sheet
          </button>
        ))}
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <div className="room" data-tab={tab}>
          <RoomHead
            project={project}
            me={asMe(isMaintainer ? mia : diego)}
            queue={queue}
            tab={tab}
            onTab={setTab}
            onStart={() => setSheet("start")}
            onEnd={() => setSheet("end")}
          />
          <YourTurnBanner queue={queue} />
          <section className="chat" aria-label="Chat">
            <p className="chat-note">The chat is not part of this preview.</p>
          </section>
          <QueuePanel
            project={project}
            members={MEMBERS}
            state={state}
            onStart={() => setSheet("start")}
            onRaise={() => setSheet("raise")}
            onDone={() => setSheet("done")}
          />
          {sheet === "start" && (
            <StartSessionSheet project={project} onClose={closeSheet} />
          )}
          {sheet === "end" && (
            <EndSessionSheet
              session={session}
              waitingCount={2}
              answering={null}
              onClose={closeSheet}
            />
          )}
          {sheet === "raise" && (
            <RaiseHandSheet
              project={project}
              session={session}
              onClose={closeSheet}
            />
          )}
          {sheet === "done" && (
            <DoneSheet
              project={project}
              hand={hand}
              isStillAnswering
              onClose={closeSheet}
            />
          )}
        </div>
      </div>
    </div>
  );
}
