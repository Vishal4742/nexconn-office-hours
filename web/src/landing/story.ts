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

export const STORY: readonly StoryStep[] = [];

/** One observer entry: a step started or stopped crossing the middle of the window. */
export type Crossing = { readonly index: number; readonly isCrossing: boolean };

/** The steps that cross the middle of the window once these entries are applied. */
export function withEntries(
  crossing: ReadonlySet<number>,
  entries: readonly Crossing[],
): ReadonlySet<number> {
  throw new Error(
    `withEntries(${crossing.size}, ${entries.length}) is not built yet`,
  );
}

/** The current step, given the steps that cross the middle of the window. */
export function nextStep(
  current: number,
  crossing: ReadonlySet<number>,
): number {
  throw new Error(`nextStep(${current}, ${crossing.size}) is not built yet`);
}
