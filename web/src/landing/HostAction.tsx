import { signInHref, useAuth } from "../auth";
import { GitHubMark } from "../components/Icons";
import { Link } from "../components/Link";

/** The page's one action (FR-AUTH-1): host office hours, or the dashboard for someone signed in. */
export function HostAction() {
  const auth = useAuth();
  if (auth.status === "signedIn") {
    return (
      <Link href="/dashboard" className="btn primary lg">
        Go to your dashboard
      </Link>
    );
  }
  return (
    <a className="btn primary lg" href={signInHref("/dashboard")}>
      <GitHubMark />
      Host office hours for your repo
    </a>
  );
}
