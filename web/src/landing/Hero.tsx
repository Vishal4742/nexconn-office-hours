import { Check } from "../components/Icons";
import { brushMaskUrl } from "./brush";
import { HostAction } from "./HostAction";

/** The painted swath the photo shows through; the same on every render. */
const BRUSH = brushMaskUrl();

/** S1: what this is in a few seconds, the one action, and the afternoon through a painted swath. */
export function Hero() {
  return (
    <section className="hero" aria-labelledby="hero-h">
      <div>
        <h1 id="hero-h">
          Answer every question{" "}
          <span className="sel">
            <span className="serif">once.</span>
            <i className="hd tl" />
            <i className="hd br" />
          </span>
        </h1>
        <p className="sub">
          Live office hours for your open source project. Contributors raise a
          hand, you take them one at a time, and every answer becomes a public
          page with its own link.
        </p>
        <div className="ctas">
          <HostAction />
        </div>
        <p className="proof">
          <span>
            <Check />
            Free for open source
          </span>
          <span>
            <Check />
            Sign in with GitHub
          </span>
          <span>
            <Check />
            Nothing to install
          </span>
        </p>
      </div>
      {/* The page's largest paint (NFR-PERF-2): a plain image, fetched first, with nothing on it. */}
      <img
        className="hero-photo land-hero-photo"
        src="/hero-meadow.webp"
        alt="A wooden desk with a lamp and a chair on a meadow, in the afternoon"
        width={1536}
        height={864}
        fetchPriority="high"
        style={{ WebkitMaskImage: BRUSH, maskImage: BRUSH }}
      />
    </section>
  );
}
