import type { KeyboardEvent } from "react";

import type { RoomTab } from "../RoomHead";

type Props = {
  readonly tab: RoomTab;
  readonly onTab: (tab: RoomTab) => void;
  /** How many hands wait, said on the queue's tab. */
  readonly waiting: number;
};

const TABS: readonly RoomTab[] = ["queue", "chat"];
const NAMES: Readonly<Record<RoomTab, string>> = {
  queue: "Queue",
  chat: "Chat",
};
const tabId = (tab: RoomTab) => `rh-tab-${tab}`;

/** The tab an arrow key, Home or End moves to, as in any row of tabs; null for other keys. */
function tabForKey(key: string, tab: RoomTab): RoomTab | null {
  if (key === "Home") return TABS[0];
  if (key === "End") return TABS[TABS.length - 1];
  const step = key === "ArrowRight" ? 1 : key === "ArrowLeft" ? -1 : 0;
  if (step === 0) return null;
  return TABS[(TABS.indexOf(tab) + step + TABS.length) % TABS.length];
}

/**
 * Below 1040 px the room shows one pane at a time, the queue first (docs/UI.md, "S3 on phones",
 * NFR-COMP-1). room.css shows the pane that `data-tab` on .room names; wider, these are hidden.
 */
export function RoomTabs({ tab, onTab, waiting }: Props) {
  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const next = tabForKey(event.key, tab);
    if (next === null) return;
    event.preventDefault();
    onTab(next);
    document.getElementById(tabId(next))?.focus();
  }

  return (
    <div
      className="rh-tabs"
      role="tablist"
      aria-label="Room sections"
      onKeyDown={handleKeyDown}
    >
      {TABS.map((name) => (
        <button
          key={name}
          id={tabId(name)}
          type="button"
          role="tab"
          aria-selected={name === tab}
          tabIndex={name === tab ? 0 : -1}
          onClick={() => onTab(name)}
        >
          {NAMES[name]}
          {name === "queue" && waiting > 0 && (
            <span className="rh-count">
              {waiting}
              <span className="sr-only"> waiting</span>
            </span>
          )}
        </button>
      ))}
    </div>
  );
}
