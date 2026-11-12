import { useEffect, useState } from "react";

import type { Connection } from "../nexconn/client";
import { loadClient } from "../nexconn/lazy";

/**
 * Connects the signed-in page to Nexconn (FR-AUTH-3) and says so when the connection is down.
 * Rendered only while someone is signed in; when it goes away the page disconnects (FR-AUTH-4).
 */
export function ConnectionBanner() {
  const [connection, setConnection] = useState<Connection>({ status: "idle" });

  useEffect(() => {
    let isCurrent = true;
    let unsubscribe = () => {};
    let leave = () => {};
    loadClient().then(
      (client) => {
        if (!isCurrent) return;
        unsubscribe = client.subscribe(() =>
          setConnection(client.getConnection()),
        );
        leave = () => void client.disconnect();
        void client.ensureConnected();
      },
      () => {
        if (isCurrent) {
          setConnection({
            status: "failed",
            message: "The chat could not be loaded. Check your connection.",
          });
        }
      },
    );
    return () => {
      isCurrent = false;
      unsubscribe();
      leave();
    };
  }, []);

  if (connection.status === "reconnecting") {
    return (
      <p className="banner" role="status">
        Reconnecting to the chat…
      </p>
    );
  }
  if (connection.status === "failed") {
    return (
      <p className="banner" role="alert">
        <span>{connection.message}</span>
        <button
          type="button"
          className="btn ghost sm"
          onClick={() =>
            void loadClient().then((client) => client.ensureConnected())
          }
        >
          Try again
        </button>
      </p>
    );
  }
  return null;
}
