import { useEffect, useState } from "react";

import { isHealthy } from "./lib/health";

type ServerState = "checking" | "up" | "down";

const MESSAGE: Record<ServerState, string> = {
  checking: "Checking the Go server…",
  up: "The Go server is answering.",
  down: "The Go server is not answering.",
};

async function checkServer(signal: AbortSignal): Promise<ServerState> {
  const res = await fetch("/api/health", { signal });
  if (!res.ok) return "down";
  const body: unknown = await res.json();
  return isHealthy(body) ? "up" : "down";
}

export default function App() {
  const [server, setServer] = useState<ServerState>("checking");

  // One-off check on load. P1 replaces this page, and data fetching then moves to a cache library.
  useEffect(() => {
    const controller = new AbortController();
    checkServer(controller.signal)
      .then(setServer)
      .catch(() => {
        if (!controller.signal.aborted) setServer("down");
      });
    return () => controller.abort();
  }, []);

  return (
    <main>
      <h1>Office Hours</h1>
      <p>
        Live office hours for open source projects. This is the P0 skeleton
        page.
      </p>
      <p role="status">{MESSAGE[server]}</p>
    </main>
  );
}
