import { useEffect, useRef, useState } from "react";

import type { Connection } from "../nexconn/client";
import { loadClient } from "../nexconn/lazy";

type Client = Awaited<ReturnType<typeof loadClient>>;

/**
 * Connects the signed-in page to Nexconn (FR-AUTH-3) and says so when the connection is down.
 * Rendered only while someone is signed in; when it goes away the page disconnects (FR-AUTH-4).
 */
export function ConnectionBanner() {
  const [connection, setConnection] = useState<Connection>({ status: "idle" });
  const [downloads, setDownloads] = useState(0);
  const clientRef = useRef<Client | null>(null);

  useEffect(() => {
    let isCurrent = true;
    let unsubscribe = () => {};
    loadClient().then(
      (client) => {
        if (!isCurrent) return;
        clientRef.current = client;
        unsubscribe = client.subscribe(() =>
          setConnection(client.getConnection()),
        );
        setConnection(client.getConnection());
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
      void clientRef.current?.disconnect();
      clientRef.current = null;
    };
  }, [downloads]);

  function handleRetry() {
    // Two things can have failed: the connection, or the download of the chat's code itself.
    if (clientRef.current) void clientRef.current.ensureConnected();
    else setDownloads((count) => count + 1);
  }

  return (
    <>
      {/* Always in the page, so a screen reader hears the text when it appears. */}
      <div role="status">
        {connection.status === "reconnecting" && (
          <p className="banner">Reconnecting to the chat…</p>
        )}
      </div>
      {connection.status === "failed" && (
        <p className="banner" role="alert">
          <span>{connection.message}</span>
          <button type="button" className="btn ghost sm" onClick={handleRetry}>
            Try again
          </button>
        </p>
      )}
    </>
  );
}
