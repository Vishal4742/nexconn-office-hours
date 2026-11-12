// The story's dots (docs/UI.md §1): the 5×7 dot-matrix clock, and the wave in which the next
// photo switches on dot by dot, out from the lamp. Plain functions; the canvas lives in StoryFrame.

export type Point = { readonly x: number; readonly y: number };
export type Size = { readonly width: number; readonly height: number };
export type Rect = Point & Size;

/** The clock's font: seven rows per character, "1" is a lit dot. */
export const DOT_FONT: Readonly<Record<string, readonly string[]>> = {};

export type ClockDot = {
  /** The dot's place on the face, the same for every time, so a change flips dots in place. */
  readonly key: string;
  readonly cx: number;
  readonly cy: number;
  readonly isOn: boolean;
  /** When this dot flips after a change, in ms: the dots flip in a scattered order. */
  readonly delay: number;
};

/** Every dot of the clock face for a time written "HH:MM". */
export function clockDots(time: string): readonly ClockDot[] {
  throw new Error(`clockDots(${time}) is not built yet`);
}

/** Where a picture drawn with `object-fit: cover` lands in a box; `position` is object-position as fractions. */
export function coverRect(box: Size, picture: Size, position: Point): Rect {
  throw new Error(
    `coverRect(${box.width}, ${picture.width}, ${position.x}) is not built yet`,
  );
}

/** A point of the picture (in the picture's own pixels), where it lands in the box. */
export function pointIn(rect: Rect, picture: Size, point: Point): Point {
  throw new Error(
    `pointIn(${rect.x}, ${picture.width}, ${point.x}) is not built yet`,
  );
}

/** Pixels between two dots of the wave. */
export const DOT_GRID = 16;
/** A full dot's radius: full dots overlap their neighbours, so together they are the whole picture. */
export const DOT_FULL = 12;
/** How long the wave takes. */
export const WAVE_MS = 1400;
/** The share of the wave the farthest dot waits before it starts to grow. */
export const WAVE_SPREAD = 0.6;

/** A dot of the wave, and when it starts to grow (0 to WAVE_SPREAD of the wave). */
export type WaveDot = Point & { readonly start: number };

/** The wave's dots over a box, in the order they switch on: nearest the lamp first. */
export function waveDots(box: Size, lamp: Point): readonly WaveDot[] {
  throw new Error(`waveDots(${box.width}, ${lamp.x}) is not built yet`);
}

/** A dot's radius at time `t` of the wave (0 to 1), given when it starts. */
export function dotRadius(t: number, start: number): number {
  throw new Error(`dotRadius(${t}, ${start}) is not built yet`);
}
