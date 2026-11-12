import { useState } from "react";

import { signInHref, signOut, useAuth } from "../auth";
import { navigate } from "../router";
import { Logo } from "./Icons";
import { Link } from "./Link";

/** The top bar of every page: the brand, and who is signed in. */
export function Header() {
  const auth = useAuth();
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [error, setError] = useState("");

  async function handleSignOut() {
    setIsSigningOut(true);
    setError("");
    try {
      await signOut();
      navigate("/");
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "Signing out failed.",
      );
    } finally {
      setIsSigningOut(false);
    }
  }

  return (
    <header className="topline">
      <Link href="/" className="brand" aria-label="Office Hours home">
        <Logo />
        <span>Office Hours</span>
      </Link>
      <div className="end">
        {error && (
          <span role="alert" className="muted">
            {error}
          </span>
        )}
        {auth.status === "signedIn" && (
          <>
            <Link href="/dashboard" className="text-btn hide-narrow">
              Dashboard
            </Link>
            <span className="who">
              <img
                className="av"
                src={auth.me.avatarUrl}
                alt=""
                width={28}
                height={28}
                referrerPolicy="no-referrer"
              />
              <span>{auth.me.name}</span>
            </span>
            <button
              type="button"
              className="btn ghost sm"
              onClick={handleSignOut}
              disabled={isSigningOut}
            >
              Sign out
            </button>
          </>
        )}
        {auth.status === "signedOut" && (
          <>
            <a className="text-btn hide-narrow" href={signInHref("/dashboard")}>
              Sign in
            </a>
            <a className="btn primary sm" href={signInHref("/dashboard")}>
              Host office hours
            </a>
          </>
        )}
      </div>
    </header>
  );
}
