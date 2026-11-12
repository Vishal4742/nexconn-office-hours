import { useEffect, useState } from "react";

import { Check, LinkIcon } from "../components/Icons";

type Props = {
  /** The page to share, as a path of this site. */
  readonly path: string;
};

type Copy = "idle" | "copied" | "failed";

const SHOWN_MS = 2000;

/** Copies the room's address, so a maintainer can post it where their contributors are. */
export function CopyLink({ path }: Props) {
  const [copy, setCopy] = useState<Copy>("idle");

  useEffect(() => {
    if (copy === "idle") return;
    const timer = setTimeout(() => setCopy("idle"), SHOWN_MS);
    return () => clearTimeout(timer);
  }, [copy]);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(window.location.origin + path);
      setCopy("copied");
    } catch {
      // The browser refused the clipboard. The address bar holds the same link.
      setCopy("failed");
    }
  }

  const said =
    copy === "copied"
      ? "Link copied"
      : copy === "failed"
        ? "Copying failed. The link is in the address bar."
        : "";
  return (
    <>
      <button
        type="button"
        className="icon-btn"
        onClick={handleCopy}
        aria-label="Copy the room's link"
        title={said || "Copy the room's link"}
      >
        {copy === "copied" ? <Check /> : <LinkIcon />}
      </button>
      <span className="sr-only" role="status">
        {said}
      </span>
    </>
  );
}
