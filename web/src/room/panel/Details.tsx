import { Fragment } from "react";

import { linkParts } from "./links";

type Props = {
  readonly text: string;
  readonly className: string;
};

/** A hand's details: plain text with its line breaks, and http or https addresses as links. */
export function Details({ text, className }: Props) {
  return (
    <p className={className}>
      {linkParts(text).map((part, at) =>
        part.kind === "link" ? (
          <a
            key={at}
            href={part.href}
            target="_blank"
            rel="noopener noreferrer nofollow"
          >
            {part.text}
          </a>
        ) : (
          <Fragment key={at}>{part.text}</Fragment>
        ),
      )}
    </p>
  );
}
