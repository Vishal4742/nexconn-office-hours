import { useEffect, useRef, useState } from "react";

import { Moment } from "./Moments";
import { StoryFrame } from "./StoryFrame";
import { PHOTOS, PHOTO_SIZE, STORY, nextStep, withEntries } from "./story";
import type { StoryStep } from "./story";

/** A band across the middle of the window: the step crossing it is the one being read. */
const MIDDLE_OF_WINDOW = "-45% 0px -45% 0px";

/**
 * "Forty five minutes, one question at a time": five moments of one session, each with the piece
 * of the product it shows, beside a frame that stays pinned while the steps scroll. No scroll
 * handler: the frame is sticky, and an observer says which step crosses the middle of the window.
 */
export function StorySection() {
  const [current, setCurrent] = useState(0);
  const stepsRef = useRef<HTMLOListElement>(null);

  useEffect(() => {
    const steps = Array.from(stepsRef.current?.children ?? []);
    let crossing: ReadonlySet<number> = new Set();
    const observer = new IntersectionObserver(
      (entries) => {
        const now = withEntries(
          crossing,
          entries.map((entry) => ({
            index: steps.indexOf(entry.target),
            isCrossing: entry.isIntersecting,
          })),
        );
        crossing = now;
        setCurrent((step) => nextStep(step, now));
      },
      { rootMargin: MIDDLE_OF_WINDOW },
    );
    steps.forEach((step) => observer.observe(step));
    return () => observer.disconnect();
  }, []);

  return (
    <section className="land-section" aria-labelledby="land-story-h">
      <h2 id="land-story-h" className="land-h2">
        Forty five minutes, one question at a time.
      </h2>
      <div className="land-story">
        <ol ref={stepsRef} className="land-steps">
          {STORY.map((step, index) => (
            <Step key={step.time} step={step} isCurrent={index === current} />
          ))}
        </ol>
        {/* The steps say everything the frame shows. */}
        <div className="land-story-media" aria-hidden="true">
          <StoryFrame step={STORY[current] ?? STORY[0]} />
        </div>
      </div>
    </section>
  );
}

type StepProps = { readonly step: StoryStep; readonly isCurrent: boolean };

function Step({ step, isCurrent }: StepProps) {
  return (
    <li className={isCurrent ? "land-step land-step-on" : "land-step"}>
      {/* On phones each step carries its own photo; beside the frame it is not drawn, nor fetched. */}
      <img
        className="land-step-photo"
        src={PHOTOS[step.photo].src}
        alt=""
        width={PHOTO_SIZE.width}
        height={PHOTO_SIZE.height}
        loading="lazy"
        decoding="async"
      />
      <time className="land-step-time" dateTime={step.time}>
        {step.time}
      </time>
      <h3 className="land-step-heading">{step.heading}</h3>
      <p className="land-step-text">{step.text}</p>
      <Moment kind={step.moment} />
    </li>
  );
}
