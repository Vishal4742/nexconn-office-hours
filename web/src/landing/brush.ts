// The hero's painted swath (docs/UI.md §1): the afternoon photo shows through thin bristle rows
// with ragged ends, slightly roughened. It is an SVG image used as a CSS mask on the photo, so the
// photo stays a plain <img> that the browser can fetch first (NFR-PERF-2). The values are the
// approved demo's; the rows are seeded, so the swath is the same on every load.

/** The swath's own coordinates; the mask is stretched over the photo's box (700:640). */
export const BRUSH_WIDTH = 700;
export const BRUSH_HEIGHT = 640;
/** No bristle comes closer than this to the photo's left or right edge. */
export const BRUSH_EDGE = 6;

const ROWS = 75;
const ROW_TOP = 20;
const ROW_PITCH = 8;
const ROW_HEIGHT = 9.5;
const SEED = 11;

/** How far the swath reaches, [left, right], at thirteen heights from top to bottom. */
const REACH: readonly (readonly [number, number])[] = [
  [60, 600],
  [30, 660],
  [18, 690],
  [26, 668],
  [12, 688],
  [22, 698],
  [8, 676],
  [24, 692],
  [14, 668],
  [30, 690],
  [18, 650],
  [40, 680],
  [70, 620],
];

/** Drops of paint beside the swath: [x, y, radius]. */
const SPLATS: readonly (readonly [number, number, number])[] = [
  [688, 566, 5],
  [672, 596, 3],
  [650, 618, 6],
  [52, 606, 4],
  [26, 584, 2.5],
  [640, 22, 3.5],
  [694, 474, 2.5],
  [8, 96, 3],
];

// In the demo the rows reached from -7 to 716 and spilled past the picture. A mask cannot paint
// outside its element, so the whole swath is narrowed to fit, ragged ends and all.
const DEMO_LEFT = -7;
const DEMO_RIGHT = 716;

function fit(x: number): number {
  const scale = (BRUSH_WIDTH - 2 * BRUSH_EDGE) / (DEMO_RIGHT - DEMO_LEFT);
  return BRUSH_EDGE + (x - DEMO_LEFT) * scale;
}

function tenth(value: number): number {
  return Math.round(value * 10) / 10;
}

/** The same numbers in the same order on every call. */
function seeded(seed: number): () => number {
  let state = seed;
  return () => {
    state = (state * 9301 + 49297) % 233280;
    return state / 233280;
  };
}

export type BrushRow = {
  readonly x: number;
  readonly y: number;
  readonly width: number;
};

/** The swath's rows, top to bottom. */
export function brushRows(): readonly BrushRow[] {
  const random = seeded(SEED);
  return Array.from({ length: ROWS }, (_, index) => {
    const at = (index / (ROWS - 1)) * (REACH.length - 1);
    const band = Math.min(REACH.length - 2, Math.floor(at));
    const share = at - band;
    const [left0, right0] = REACH[band] ?? [0, 0];
    const [left1, right1] = REACH[band + 1] ?? [0, 0];
    // The order of the random() calls is the demo's: it decides the shape.
    const left =
      left0 +
      (left1 - left0) * share +
      (random() - 0.5) * 30 +
      (random() < 0.12 ? random() * 70 : 0);
    const right =
      right0 +
      (right1 - right0) * share +
      (random() - 0.5) * 36 -
      (random() < 0.14 ? random() * 100 : 0);
    const x = tenth(fit(left));
    return {
      x,
      y: ROW_TOP + index * ROW_PITCH,
      width: tenth(fit(right) - x),
    };
  });
}

function brushSvg(): string {
  const rows = brushRows()
    .map(
      (row) =>
        `<rect x="${row.x}" y="${row.y}" width="${row.width}" height="${ROW_HEIGHT}" rx="4"/>`,
    )
    .join("");
  const splats = SPLATS.map(
    ([x, y, radius]) =>
      `<circle cx="${tenth(fit(x))}" cy="${y}" r="${radius}"/>`,
  ).join("");
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${BRUSH_WIDTH} ${BRUSH_HEIGHT}" preserveAspectRatio="none">` +
    `<filter id="rough" x="-8%" y="-8%" width="116%" height="116%">` +
    `<feTurbulence type="fractalNoise" baseFrequency=".03 .2" numOctaves="2" seed="5" result="noise"/>` +
    `<feDisplacementMap in="SourceGraphic" in2="noise" scale="9" xChannelSelector="R" yChannelSelector="G"/>` +
    `</filter>` +
    `<g fill="#fff" filter="url(#rough)">${rows}${splats}</g></svg>`
  );
}

/** The swath as a CSS image value: url("data:image/svg+xml,…"). */
export function brushMaskUrl(): string {
  return `url("data:image/svg+xml,${encodeURIComponent(brushSvg())}")`;
}
