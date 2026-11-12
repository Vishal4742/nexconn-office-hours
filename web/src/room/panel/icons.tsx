// The queue panel's own icons, inline like components/Icons.tsx. All are decorative: the text
// next to each one says what it means.
import type { ReactNode } from "react";

function Stroke({ children }: { readonly children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 16 16"
      aria-hidden="true"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {children}
    </svg>
  );
}

export function HandIcon() {
  return (
    <Stroke>
      <path d="M5.5 8.5V4a1 1 0 0 1 2 0v3.5M7.5 7.5V3a1 1 0 0 1 2 0v4.5M9.5 7.5V4a1 1 0 0 1 2 0v4M11.5 8V6.5a1 1 0 0 1 2 0V10a5 5 0 0 1-5 5H8a4.5 4.5 0 0 1-3.6-1.8l-1.8-2.4a1 1 0 0 1 1.6-1.2l1.3 1.6V8.5" />
    </Stroke>
  );
}

export function NextIcon() {
  return (
    <Stroke>
      <path d="M3.5 3.5v9l7-4.5zM12.5 3.5v9" />
    </Stroke>
  );
}

export function StarIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" fill="currentColor">
      <path d="m8 1.6 1.95 3.95 4.35.63-3.15 3.07.74 4.33L8 11.53l-3.89 2.05.74-4.33L1.7 6.18l4.35-.63z" />
    </svg>
  );
}

export function DotsIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" fill="currentColor">
      <circle cx="3.5" cy="8" r="1.25" />
      <circle cx="8" cy="8" r="1.25" />
      <circle cx="12.5" cy="8" r="1.25" />
    </svg>
  );
}
