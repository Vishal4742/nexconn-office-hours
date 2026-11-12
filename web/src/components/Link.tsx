import type { MouseEvent, ReactNode } from "react";

import { navigate } from "../router";

type Props = {
  readonly href: string;
  readonly className?: string;
  readonly children: ReactNode;
  readonly "aria-label"?: string;
};

/** A link to another page of the app. A plain click changes the page without a reload. */
export function Link({ href, className, children, ...rest }: Props) {
  function handleClick(event: MouseEvent<HTMLAnchorElement>) {
    // Let the browser handle new-tab clicks and anything a handler upstream already took.
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    navigate(href);
  }
  return (
    <a href={href} className={className} onClick={handleClick} {...rest}>
      {children}
    </a>
  );
}
