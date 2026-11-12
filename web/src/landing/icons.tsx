// The landing's few icons, inline. All are decorative: the text beside each says what it means.

const STROKE = {
  fill: "none",
  stroke: "currentColor",
  strokeLinecap: "round",
  strokeLinejoin: "round",
} as const;

export function Bulb() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" strokeWidth={1.6} {...STROKE}>
      <path d="M9.5 18h5M10.5 21h3" />
      <path d="M12 3a6 6 0 0 0-3.6 10.8c.7.5 1.1 1.3 1.1 2.2v.5h5V16c0-.9.4-1.7 1.1-2.2A6 6 0 0 0 12 3Z" />
    </svg>
  );
}

export function RaisedHand() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" strokeWidth={1.6} {...STROKE}>
      <path d="M8 13V5.5a1.5 1.5 0 0 1 3 0V11M11 11V4a1.5 1.5 0 0 1 3 0v7M14 11V5.5a1.5 1.5 0 0 1 3 0V13" />
      <path d="M17 13V8.5a1.5 1.5 0 0 1 3 0V15a7 7 0 0 1-7 7h-1a6 6 0 0 1-4.8-2.4l-3.6-4.8a1.6 1.6 0 0 1 2.4-2.1L8 15.5V13" />
    </svg>
  );
}

export function Bookmark() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" strokeWidth={1.5} {...STROKE}>
      <path d="M4 2.5h8v11l-4-3-4 3z" />
    </svg>
  );
}

export function ArrowUpRight() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" strokeWidth={1.5} {...STROKE}>
      <path d="M5 11 11 5M6 5h5v5" />
    </svg>
  );
}

export function Caret() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" strokeWidth={1.5} {...STROKE}>
      <path d="m4 6 4 4 4-4" />
    </svg>
  );
}
