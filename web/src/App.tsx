import { useAuth } from "./auth";
import { ConnectionBanner } from "./components/ConnectionBanner";
import { Header } from "./components/Header";
import { Dashboard } from "./pages/Dashboard";
import { Landing } from "./pages/Landing";
import { NotFound } from "./pages/NotFound";
import { Room } from "./pages/Room";
import { useRoute } from "./router";

export default function App() {
  const route = useRoute();
  const auth = useAuth();
  // The room fills the window and has a top row of its own (docs/UI.md S3).
  const isRoom = route.name === "room";

  return (
    <div className={isRoom ? "app app-room" : "app"}>
      <a className="skip" href="#main">
        Skip to the page
      </a>
      {!isRoom && <Header />}
      {auth.status === "signedIn" && <ConnectionBanner />}
      <main id="main">
        {route.name === "landing" && <Landing />}
        {route.name === "dashboard" && <Dashboard />}
        {route.name === "room" && (
          <Room
            key={`${route.owner}/${route.repo}`.toLowerCase()}
            owner={route.owner}
            repo={route.repo}
          />
        )}
        {route.name === "notFound" && <NotFound />}
      </main>
    </div>
  );
}
