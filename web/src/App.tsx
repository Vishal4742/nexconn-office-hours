import { Suspense, lazy, useEffect, useRef } from "react";

import { useAuth } from "./auth";
import { ConnectionBanner } from "./components/ConnectionBanner";
import { Header } from "./components/Header";
import { Dashboard } from "./pages/Dashboard";
import { Landing } from "./pages/Landing";
import { NotFound } from "./pages/NotFound";
import { Room } from "./pages/Room";
import { Stats } from "./pages/Stats";
import { titleFor, useRoute } from "./router";

// Development only: the room's parts in every state, from fixtures. import.meta.env.DEV is false
// in a production build, so neither the page nor its file is part of one.
const DevRoom = import.meta.env.DEV
  ? lazy(() =>
      import("./pages/DevRoom").then((page) => ({ default: page.DevRoom })),
    )
  : null;

export default function App() {
  const route = useRoute();
  const auth = useAuth();
  // The room fills the window and has a top row of its own (docs/UI.md S3).
  const isRoom = route.name === "room" || route.name === "devRoom";
  const mainRef = useRef<HTMLElement>(null);
  const shownPath = useRef(window.location.pathname);

  // NFR-A11Y-1: a change of page without a reload is silent to a screen reader and leaves the
  // keyboard on a link that is gone. So the tab gets the page's title and focus moves to the page.
  useEffect(() => {
    document.title = titleFor(route);
    if (shownPath.current === window.location.pathname) return;
    shownPath.current = window.location.pathname;
    mainRef.current?.focus();
  }, [route]);

  return (
    <div className={isRoom ? "app app-room" : "app"}>
      <a className="skip" href="#main">
        Skip to the page
      </a>
      {!isRoom && <Header />}
      {auth.status === "signedIn" && <ConnectionBanner />}
      <main id="main" ref={mainRef} tabIndex={-1}>
        {route.name === "landing" && <Landing />}
        {route.name === "dashboard" && <Dashboard />}
        {route.name === "room" && (
          <Room
            key={`${route.owner}/${route.repo}`.toLowerCase()}
            owner={route.owner}
            repo={route.repo}
          />
        )}
        {route.name === "stats" && <Stats repo={null} />}
        {route.name === "projectStats" && (
          <Stats
            key={`${route.owner}/${route.repo}`.toLowerCase()}
            repo={{ owner: route.owner, repo: route.repo }}
          />
        )}
        {route.name === "devRoom" && DevRoom && (
          <Suspense fallback={null}>
            <DevRoom />
          </Suspense>
        )}
        {route.name === "notFound" && <NotFound />}
      </main>
    </div>
  );
}
