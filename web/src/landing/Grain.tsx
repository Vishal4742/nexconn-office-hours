type Props = {
  /** Unique in the page: the filter is found by it. */
  readonly id: string;
  /** Lightens instead of darkening, for the night photo of the closing. */
  readonly isLight?: boolean;
};

/** Light film grain over a photo (docs/UI.md §1). Drawn once; it never moves. */
export function Grain({ id, isLight = false }: Props) {
  return (
    <svg
      className={isLight ? "land-grain land-grain-light" : "land-grain"}
      aria-hidden="true"
    >
      <filter id={id}>
        <feTurbulence
          type="fractalNoise"
          baseFrequency="0.85"
          numOctaves={2}
          stitchTiles="stitch"
        />
        <feColorMatrix type="saturate" values="0" />
        <feComponentTransfer>
          <feFuncA type="linear" slope={0.16} />
        </feComponentTransfer>
      </filter>
      <rect width="100%" height="100%" filter={`url(#${id})`} />
    </svg>
  );
}
