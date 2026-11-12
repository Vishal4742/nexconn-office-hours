import { Fragment } from "react";

const SENTENCE =
  "The same question should never cost you two evenings. Answer it live once, and it becomes a link.";
const WORDS = SENTENCE.split(" ");
const BIRDS = [
  "M1060 120q10-8 20 0q10-8 20 0",
  "M1110 150q7-5.6 14 0q7-5.6 14 0",
  "M990 168q5.5-4.4 11 0q5.5-4.4 11 0",
];

/**
 * The misty sky band with the one sentence (docs/UI.md S1), drawn in SVG. Its words light up as
 * they scroll into view where the browser can tie an animation to scrolling (landing.css).
 */
export function Sky() {
  return (
    <div className="land-sky">
      <svg
        className="land-sky-art"
        viewBox="0 0 1440 520"
        preserveAspectRatio="xMidYMid slice"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="land-sky-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" className="land-sky-top" />
            <stop offset="1" className="land-sky-low" />
          </linearGradient>
          <filter id="land-sky-clouds" x="0" y="0" width="100%" height="100%">
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.0026 0.0082"
              numOctaves={5}
              seed={14}
            />
            <feColorMatrix values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  2.3 0 0 0 -1.1" />
          </filter>
          <linearGradient id="land-sky-fade" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#fff" stopOpacity={0.25} />
            <stop offset="0.8" stopColor="#fff" />
          </linearGradient>
          <mask id="land-sky-mask">
            <rect width="1440" height="520" fill="url(#land-sky-fade)" />
          </mask>
          <filter id="land-sky-grain">
            <feTurbulence
              type="fractalNoise"
              baseFrequency="0.9"
              numOctaves={2}
              stitchTiles="stitch"
            />
            <feColorMatrix type="saturate" values="0" />
            <feComponentTransfer>
              <feFuncA type="linear" slope={0.12} />
            </feComponentTransfer>
          </filter>
        </defs>
        <rect width="1440" height="520" fill="url(#land-sky-fill)" />
        <rect
          className="land-sky-clouds"
          width="1440"
          height="520"
          filter="url(#land-sky-clouds)"
          mask="url(#land-sky-mask)"
        />
        <g className="land-sky-birds">
          {BIRDS.map((bird) => (
            <path key={bird} d={bird} />
          ))}
        </g>
        <rect
          className="land-sky-grain"
          width="1440"
          height="520"
          filter="url(#land-sky-grain)"
        />
      </svg>
      <p>
        {WORDS.map((word, index) => (
          <Fragment key={index}>
            {index > 0 && " "}
            <span
              className={word === "once," ? "land-word serif" : "land-word"}
            >
              {word}
            </span>
          </Fragment>
        ))}
      </p>
    </div>
  );
}
