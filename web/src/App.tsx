import { Suspense, lazy } from "react";

import { Header } from "./components/Header";
import { Dashboard } from "./pages/Dashboard";
import { Landing } from "./pages/Landing";
import { NotFound } from "./pages/NotFound";
import { useRoute } from "./router";

// P0 only: the Chat UI test page loads the Nexconn SDK on demand. It goes when the real room lands.
const SpikePage = lazy(() => import("./spike/SpikePage"));

export default function App() {
  const route = useRoute();

  if (route.name === "spike") {
    return (
      <Suspense fallback={<p>Loading the chat…</p>}>
        <SpikePage />
      </Suspense>
    );
  }
  return (
    <>
      <a className="skip" href="#main">
        Skip to the page
      </a>
      <Header />
      <main id="main">
        {route.name === "landing" && <Landing />}
        {route.name === "dashboard" && <Dashboard />}
        {/* The project page arrives with the room slices. */}
        {(route.name === "room" || route.name === "notFound") && <NotFound />}
      </main>
    </>
  );
}
