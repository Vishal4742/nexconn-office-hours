import { retryAuth, signInHref, useAuth } from "../auth";
import { GitHubMark } from "../components/Icons";

/** S2: my projects. The list and the form to register a repo arrive with the projects slice. */
export function Dashboard() {
  const auth = useAuth();

  if (auth.status === "loading") {
    return (
      <div className="wrap page" aria-busy="true">
        <h1>Your projects</h1>
        <p className="empty">Loading…</p>
      </div>
    );
  }
  if (auth.status === "unreachable") {
    return (
      <div className="wrap page">
        <h1>Your projects</h1>
        <p className="empty" role="alert">
          {auth.message}
        </p>
        <div className="actions">
          <button type="button" className="btn ghost" onClick={retryAuth}>
            Try again
          </button>
        </div>
      </div>
    );
  }
  if (auth.status === "signedOut") {
    return (
      <div className="wrap page">
        <h1>Your projects</h1>
        <p className="lead">
          Sign in with GitHub to host office hours for a repo you maintain, or
          to see the rooms you joined.
        </p>
        <div className="actions">
          <a className="btn primary lg" href={signInHref("/dashboard")}>
            <GitHubMark />
            Sign in with GitHub
          </a>
        </div>
      </div>
    );
  }
  return (
    <div className="wrap page">
      <h1>Your projects</h1>
      <p className="lead">Signed in as @{auth.me.login}.</p>
      <p className="empty">No projects yet.</p>
    </div>
  );
}
