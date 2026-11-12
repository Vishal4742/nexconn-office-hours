import { Grain } from "./Grain";
import { HostAction } from "./HostAction";

/** After hours (docs/UI.md §1): the night photo with its colour fringe and light grain, and the action again. */
export function Closing() {
  return (
    <section className="land-closing" aria-labelledby="land-closing-h">
      <img
        className="land-closing-photo"
        src="/night-ca.webp"
        alt="The same desk at night, the lamp still on"
        width={1536}
        height={864}
        loading="lazy"
        decoding="async"
      />
      <Grain id="land-grain-closing" isLight />
      <div className="land-closing-text">
        <h2 id="land-closing-h">
          See you <span className="serif">Thursday.</span>
        </h2>
        <p>
          Your next office hours start when you say so. Free for open source,
          about two minutes to set up.
        </p>
        <div className="land-closing-action">
          <HostAction />
        </div>
      </div>
    </section>
  );
}
