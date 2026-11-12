// The landing's story (docs/UI.md §1 v3, S1): one session by the clock, while the same scene
// moves from afternoon to night. Data and plain functions only; the page is in Story.tsx.

import type { Point, Size } from "./dots";

/** The four photos of the same scene, one per hour of the session. */
export type Photo = "afternoon" | "golden" | "dusk" | "night";

export type PhotoInfo = {
  readonly src: string;
  /** The hour's name, shown under the story's clock. */
  readonly hour: string;
  /** The lamp, in the photo's own pixels: the dot wave starts there. */
  readonly lamp: Point;
  /** Dusk and night: the last photo dims first and the dots glow, like windows lighting up. */
  readonly glows: boolean;
};

/** Every photo of the scene is this size. */
export const PHOTO_SIZE: Size = { width: 1536, height: 864 };

/** The frame's crop, as object-position fractions: keep it equal to `.land-frame-photo` in landing.css. */
export const FRAME_CROP: Point = { x: 0.68, y: 0.6 };

export const PHOTOS: Readonly<Record<Photo, PhotoInfo>> = {
  afternoon: {
    src: "/hero-meadow.webp",
    hour: "afternoon",
    lamp: { x: 1050, y: 484 },
    glows: false,
  },
  golden: {
    src: "/golden.webp",
    hour: "golden hour",
    lamp: { x: 1063, y: 534 },
    glows: false,
  },
  dusk: {
    src: "/dusk.webp",
    hour: "dusk",
    lamp: { x: 1060, y: 547 },
    glows: true,
  },
  night: {
    src: "/night.webp",
    hour: "night",
    lamp: { x: 1113, y: 526 },
    glows: true,
  },
};

/** The piece of product UI a step shows (Moments.tsx). */
export type Moment = "started" | "hand" | "turn" | "answered" | "ended";

export type StoryStep = {
  /** "HH:MM", drawn by the dot-matrix clock. */
  readonly time: string;
  readonly photo: Photo;
  readonly heading: string;
  readonly text: string;
  readonly moment: Moment;
};

// Each sentence says only what the product does in this build (docs/SRS.md): FR-SESS-3, FR-HAND-1
// and 3, FR-QUEUE-5 and 9, FR-FAQ-1 to 3 with FR-HAND-8, FR-SESS-6 and 7.
export const STORY: readonly StoryStep[] = [
  {
    time: "17:00",
    photo: "afternoon",
    heading: "Doors open",
    text: "You start the session. Everyone who joined the room gets an @all card and an unread badge.",
    moment: "started",
  },
  {
    time: "17:04",
    photo: "afternoon",
    heading: "A hand goes up",
    text: "Contributors ask in one sentence. Answers to similar questions show up first, so repeats often answer themselves.",
    moment: "hand",
  },
  {
    time: "17:12",
    photo: "golden",
    heading: "It's your turn",
    text: "A banner tells you the moment your question is taken. Press I'm here and the maintainer knows you're listening.",
    moment: "turn",
  },
  {
    time: "17:31",
    photo: "dusk",
    heading: "Answered once",
    text: "Done turns two lines of answer into a public page with its own link. Everyone who asked the same thing gets it.",
    moment: "answered",
  },
  {
    time: "17:45",
    photo: "night",
    heading: "Last call",
    text: "Hands close. Whoever is still waiting moves to the front of Thursday's line, so nobody has to ask twice.",
    moment: "ended",
  },
];

/** One observer entry: a step started or stopped crossing the middle of the window. */
export type Crossing = { readonly index: number; readonly isCrossing: boolean };

/** The steps that cross the middle of the window once these entries are applied. */
export function withEntries(
  crossing: ReadonlySet<number>,
  entries: readonly Crossing[],
): ReadonlySet<number> {
  const next = new Set(crossing);
  for (const entry of entries) {
    if (entry.isCrossing) next.add(entry.index);
    else next.delete(entry.index);
  }
  return next;
}

/**
 * The current step, given the steps that cross the middle of the window. It stays while it still
 * crosses, so a reader resting on a boundary sees no flicker; otherwise the crossing step nearest
 * to it wins, which after a jump is the one the reader passed last.
 */
export function nextStep(
  current: number,
  crossing: ReadonlySet<number>,
): number {
  if (crossing.size === 0 || crossing.has(current)) return current;
  const [nearest] = [...crossing].sort(
    (a, b) => Math.abs(a - current) - Math.abs(b - current) || a - b,
  );
  return nearest ?? current;
}
