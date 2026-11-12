import { StrictMode, Suspense, lazy } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";

// P0 only: the Chat UI spike lives at /spike and loads the Nexconn SDK on demand. A router comes in P1.
const SpikePage = lazy(() => import("./spike/SpikePage"));

const root = document.getElementById("root");
if (!root) throw new Error("index.html is missing the #root element");

createRoot(root).render(
  <StrictMode>
    {window.location.pathname === "/spike" ? (
      <Suspense fallback={<p>Loading the chat…</p>}>
        <SpikePage />
      </Suspense>
    ) : (
      <App />
    )}
  </StrictMode>,
);
