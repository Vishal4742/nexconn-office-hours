// The hero's painted swath (docs/UI.md §1): the afternoon photo shows through thin bristle rows
// with ragged ends, slightly roughened. It is an SVG image used as a CSS mask on the photo, so the
// photo stays a plain <img> that the browser can fetch first (NFR-PERF-2).

/** The swath's own coordinates; the mask is stretched over the photo's box (700:640). */
export const BRUSH_WIDTH = 700;
export const BRUSH_HEIGHT = 640;
/** No bristle comes closer than this to the photo's left or right edge. */
export const BRUSH_EDGE = 6;

export type BrushRow = {
  readonly x: number;
  readonly y: number;
  readonly width: number;
};

/** The swath's rows, top to bottom. */
export function brushRows(): readonly BrushRow[] {
  throw new Error("brushRows is not built yet");
}

/** The swath as a CSS image value: url("data:image/svg+xml,…"). */
export function brushMaskUrl(): string {
  throw new Error("brushMaskUrl is not built yet");
}
