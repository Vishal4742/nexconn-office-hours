import { useLayoutEffect } from "react";

import { signInHref, signInMessage } from "../auth";
import { GitHubMark } from "../components/Icons";
import { Closing } from "../landing/Closing";
import { Facts } from "../landing/Facts";
import { Hero } from "../landing/Hero";
import { Questions } from "../landing/Questions";
import { Sky } from "../landing/Sky";
import { StorySection } from "../landing/StorySection";

/**
 * S1 (docs/UI.md): what Office Hours is in twenty seconds, then one session hour by hour, three
 * numbers, the questions a maintainer asks first, and the action again.
 */
export function Landing() {
  const failure = signInMessage(window.location.search);

  // Dark mode belongs to this page alone (docs/UI.md §1): its tokens apply under this class, which
  // is on <html> only while the page is shown. Set before paint, so no light frame shows first.
  useLayoutEffect(() => {
    const root = document.documentElement;
    root.classList.add("land-page");
    return () => root.classList.remove("land-page");
  }, []);

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
      <Hero />
      <StorySection />
      <Facts />
      <Sky />
      <Questions />
      <Closing />
      <footer className="foot">
        <span>Office Hours · built on Nexconn Chat</span>
      </footer>
    </div>
  );
}
