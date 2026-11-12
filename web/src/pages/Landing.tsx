import { signInHref, signInMessage, useAuth } from "../auth";
import { Check, GitHubMark } from "../components/Icons";
import { Link } from "../components/Link";

/** S1: say what this is in a few seconds, and offer the one action. Art and the story come in P4. */
export function Landing() {
  const auth = useAuth();
  const failure = signInMessage(window.location.search);

  return (
    <div className="wrap">
      {failure && (
        <p className="notice" role="alert">
          <span>{failure}</span>
          <a className="btn ghost sm" href={signInHref("/dashboard")}>
            <GitHubMark />
            Try again
          </a>
        </p>
      )}
      <section className="hero" aria-labelledby="hero-h">
        <div>
          <h1 id="hero-h">
            Answer every question{" "}
            <span className="sel">
              <span className="serif">once.</span>
              <i className="hd tl" />
              <i className="hd br" />
            </span>
          </h1>
          <p className="sub">
            Live office hours for your open source project. Contributors raise a
            hand, you take them one at a time, and every answer becomes a public
            page with its own link.
          </p>
          <div className="ctas">
            {auth.status === "signedIn" ? (
              <Link href="/dashboard" className="btn primary lg">
                Go to your dashboard
              </Link>
            ) : (
              <a className="btn primary lg" href={signInHref("/dashboard")}>
                <GitHubMark />
                Host office hours for your repo
              </a>
            )}
          </div>
          <p className="proof">
            <span>
              <Check />
              Free for open source
            </span>
            <span>
              <Check />
              Sign in with GitHub
            </span>
            <span>
              <Check />
              Nothing to install
            </span>
          </p>
        </div>
        <img
          className="hero-photo"
          src="/hero-meadow.webp"
          alt="A wooden desk with a lamp and a chair on a meadow, in the afternoon"
          width={1536}
          height={864}
        />
      </section>
      <footer className="foot">
        <span>Office Hours · built on Nexconn Chat</span>
      </footer>
    </div>
  );
}
