import { useEffect, useRef, useSyncExternalStore } from "react";
import type { NCChatUIApplication } from "@nexconn/chatui";

import { SPIKE_GROUP, getState, startChat, subscribe } from "./chat";
import "./spike.css";

const USERS: Readonly<Record<string, string>> = {
  a: "ohspikea",
  b: "ohspikeb",
};

type MountMode = "parts" | "detail";

/** Puts Chat UI into the host once. "parts" drops Chat UI's own header (A-9 follow-up). */
function mount(
  host: HTMLElement,
  app: NCChatUIApplication,
  mode: MountMode,
): void {
  if (host.childElementCount > 0) return;
  if (mode === "detail") {
    host.append(document.createElement("nc-channel-detail-provider"));
    return;
  }
  const toast = document.createElement("nc-toast-provider");
  const list = document.createElement("nc-message-list-provider");
  list.className = "nc-chatui-msg-list";
  const input: HTMLElement & { model?: unknown } =
    document.createElement("nc-input-provider");
  input.model = app.getOpenedChannel();
  host.append(toast, list, input);
}

export default function SpikePage() {
  const state = useSyncExternalStore(subscribe, getState);
  const hostRef = useRef<HTMLDivElement>(null);
  const params = new URLSearchParams(window.location.search);
  const user = USERS[params.get("user") ?? "a"] ?? USERS.a;
  const mode: MountMode = params.get("mount") === "detail" ? "detail" : "parts";
  const isOpen = params.get("open") !== "0"; // ?open=0 connects without opening the room

  useEffect(() => {
    let isCurrent = true;
    startChat(user, isOpen).then(
      (app) => {
        if (isOpen && isCurrent && hostRef.current)
          mount(hostRef.current, app, mode);
      },
      () => {
        // The failure is already on screen through the store.
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [user, mode, isOpen]);

  return (
    <main className="spike">
      <section
        className={`spike-chat spike-${mode}`}
        ref={hostRef}
        aria-label="Chat"
      />
      <aside className="spike-panel">
        <h1>Chat UI spike</h1>
        <dl>
          <dt>User</dt>
          <dd>{state.userId || user}</dd>
          <dt>Room</dt>
          <dd>{SPIKE_GROUP}</dd>
          <dt>State</dt>
          <dd role="status">
            {state.phase === "failed" ? `failed: ${state.error}` : state.phase}
          </dd>
          <dt>Connection</dt>
          <dd>{state.connection}</dd>
          <dt>Mount</dt>
          <dd>
            {mode === "parts"
              ? "message list + input only"
              : "channel detail (with header)"}
          </dd>
          <dt>Cards received live</dt>
          <dd>{state.ohMessages}</dd>
          <dt>Queue pings</dt>
          <dd>{state.pings}</dd>
          <dt>Last ping</dt>
          <dd>{state.lastPing}</dd>
          <dt>Forged OH messages ignored</dt>
          <dd>{state.ignored}</dd>
          <dt>Unread (room closed)</dt>
          <dd>{state.unread}</dd>
        </dl>
        <p>
          Open this page twice, with <code>?user=a</code> and{" "}
          <code>?user=b</code>, then run the server spike to send cards and a
          ping.
        </p>
      </aside>
    </main>
  );
}
