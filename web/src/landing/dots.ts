// The story's dots (docs/UI.md §1): the 5×7 dot-matrix clock, and the wave in which the next
// photo switches on dot by dot, out from the lamp. Plain functions; the canvas lives in StoryFrame.

export type Point = { readonly x: number; readonly y: number };
export type Size = { readonly width: number; readonly height: number };
export type Rect = Point & Size;

/** The clock's font: seven rows per character, "1" is a lit dot. */
export const DOT_FONT: Readonly<Record<string, readonly string[]>> = {
  "0": ["01110", "10001", "10011", "10101", "11001", "10001", "01110"],
  "1": ["00100", "01100", "00100", "00100", "00100", "00100", "01110"],
  "2": ["01110", "10001", "00001", "00010", "00100", "01000", "11111"],
  "3": ["01110", "10001", "00001", "00110", "00001", "10001", "01110"],
  "4": ["00010", "00110", "01010", "10010", "11111", "00010", "00010"],
  "5": ["11111", "10000", "11110", "00001", "00001", "10001", "01110"],
  "6": ["00110", "01000", "10000", "11110", "10001", "10001", "01110"],
  "7": ["11111", "00001", "00010", "00100", "01000", "01000", "01000"],
  "8": ["01110", "10001", "10001", "01110", "10001", "10001", "01110"],
  "9": ["01110", "10001", "10001", "01111", "00001", "00010", "01100"],
  ":": ["0", "0", "1", "0", "1", "0", "0"],
};

const BLANK = ["00000", "00000", "00000", "00000", "00000", "00000", "00000"];
/** Pixels between dot centres on the clock face (its viewBox is 200 × 56). */
const CLOCK_PITCH = 8;
/** Empty columns between two characters. */
const CLOCK_GAP = 1;
/** The longest a dot waits before it flips, in ms. */
const FLIP_SPREAD = 220;

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
  let left = 0;
  return [...time].flatMap((char, k) => {
    const rows = DOT_FONT[char] ?? BLANK;
    const first = left;
    left += (rows[0]?.length ?? 0) + CLOCK_GAP;
    return rows.flatMap((row, j) =>
      [...row].map((bit, i) => ({
        key: `${k}-${j}-${i}`,
        cx: (first + i) * CLOCK_PITCH + CLOCK_PITCH / 2,
        cy: j * CLOCK_PITCH + CLOCK_PITCH / 2,
        isOn: bit === "1",
        delay: (i * 37 + j * 53 + k * 71) % FLIP_SPREAD,
      })),
    );
  });
}

/** Where a picture drawn with `object-fit: cover` lands in a box; `position` is object-position as fractions. */
export function coverRect(box: Size, picture: Size, position: Point): Rect {
  const scale = Math.max(
    box.width / picture.width,
    box.height / picture.height,
  );
  const width = picture.width * scale;
  const height = picture.height * scale;
  return {
    x: (box.width - width) * position.x,
    y: (box.height - height) * position.y,
    width,
    height,
  };
}

/** A point of the picture (in the picture's own pixels), where it lands in the box. */
export function pointIn(rect: Rect, picture: Size, point: Point): Point {
  return {
    x: rect.x + (point.x / picture.width) * rect.width,
    y: rect.y + (point.y / picture.height) * rect.height,
  };
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
  // One row and one column past the edges, so full dots reach the box's last pixels.
  const points: Point[] = [];
  for (let y = DOT_GRID / 2; y < box.height + DOT_GRID; y += DOT_GRID) {
    for (let x = DOT_GRID / 2; x < box.width + DOT_GRID; x += DOT_GRID) {
      points.push({ x, y });
    }
  }
  const distances = points.map((point) =>
    Math.hypot(point.x - lamp.x, point.y - lamp.y),
  );
  const farthest = Math.max(...distances) || 1;
  return points
    .map((point, index) => ({
      ...point,
      start: ((distances[index] ?? 0) / farthest) * WAVE_SPREAD,
    }))
    .sort((a, b) => a.start - b.start);
}

/** A dot's radius at time `t` of the wave (0 to 1), given when it starts. */
export function dotRadius(t: number, start: number): number {
  const grown = (t - start) / (1 - WAVE_SPREAD);
  if (grown <= 0) return 0;
  const g = Math.min(grown, 1);
  return DOT_FULL * g * g * (3 - 2 * g); // smoothstep: eases in and out
}
