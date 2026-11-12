import { useEffect, useRef, useState } from "react";

import {
  WAVE_MS,
  clockDots,
  coverRect,
  dotRadius,
  pointIn,
  waveDots,
} from "./dots";
import type { Point } from "./dots";
import { Grain } from "./Grain";
import { FRAME_CROP, PHOTOS, PHOTO_SIZE } from "./story";
import type { Photo, StoryStep } from "./story";

const HOURS: readonly Photo[] = ["afternoon", "golden", "dusk", "night"];
/** Sharper than this costs pixels nobody sees while the wave moves. */
const MAX_PIXEL_RATIO = 2;

type Props = { readonly step: StoryStep };

/**
 * The frame pinned beside the story: the scene at the step's hour and the dot-matrix clock. When
 * the hour changes, the next photo switches on dot by dot in a wave out from the lamp, then the
 * picture is whole (docs/UI.md §1). With reduced motion, or with the frame off screen, it swaps.
 */
export function StoryFrame({ step }: Props) {
  const target = step.photo;
  const [shown, setShown] = useState<Photo>(target);
  const frameRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const isOnScreen = useRef(false);

  useEffect(() => {
    const frame = frameRef.current;
    if (!frame) return;
    const observer = new IntersectionObserver(([entry]) => {
      isOnScreen.current = entry?.isIntersecting ?? false;
    });
    observer.observe(frame);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (target === shown) return;
    const show = () => setShown(target);
    const canvas = canvasRef.current;
    const next = frameRef.current?.querySelector<HTMLImageElement>(
      `img[data-photo="${target}"]`,
    );
    if (!canvas || !next || !mayAnimate(next) || !isOnScreen.current) {
      const frame = requestAnimationFrame(show);
      return () => cancelAnimationFrame(frame);
    }
    return playWave(
      canvas,
      next,
      PHOTOS[target].lamp,
      () => isOnScreen.current,
      show,
    );
  }, [target, shown]);

  const glows = target !== shown && PHOTOS[target].glows;
  return (
    <div
      ref={frameRef}
      className={glows ? "land-frame land-frame-glow" : "land-frame"}
    >
      {HOURS.map((photo) => (
        <img
          key={photo}
          data-photo={photo}
          className={
            photo === shown
              ? "land-frame-photo land-frame-shown"
              : "land-frame-photo"
          }
          src={PHOTOS[photo].src}
          alt=""
          width={PHOTO_SIZE.width}
          height={PHOTO_SIZE.height}
          loading="lazy"
          decoding="async"
          // On a tall window the frame's top is in view at load: the hero photo goes first.
          fetchPriority="low"
        />
      ))}
      <div className="land-dots">
        <canvas ref={canvasRef} />
      </div>
      <Grain id="land-grain-frame" />
      <div className="land-clock">
        <DotClock time={step.time} />
        <span className="land-clock-hour">{PHOTOS[target].hour}</span>
      </div>
    </div>
  );
}

/** The step's time on a 5×7 dot-matrix face; on a change the dots flip in a scattered order. */
function DotClock({ time }: { readonly time: string }) {
  return (
    <svg className="land-clock-face" viewBox="0 0 200 56" aria-hidden="true">
      {clockDots(time).map((dot) => (
        <circle
          key={dot.key}
          className={dot.isOn ? "land-clock-on" : undefined}
          cx={dot.cx}
          cy={dot.cy}
          r={2.9}
          style={{ transitionDelay: `${dot.delay}ms` }}
        />
      ))}
    </svg>
  );
}

function mayAnimate(next: HTMLImageElement): boolean {
  return (
    next.complete &&
    next.naturalWidth > 0 &&
    !window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Draws `next` as dots that grow out from the lamp, then calls `onDone`; it stops early when the
 * frame leaves the screen. Returns a function that stops it and clears the canvas.
 */
function playWave(
  canvas: HTMLCanvasElement,
  next: HTMLImageElement,
  lamp: Point,
  isOnScreen: () => boolean,
  onDone: () => void,
): () => void {
  const context = canvas.getContext("2d");
  const box = { width: canvas.clientWidth, height: canvas.clientHeight };
  if (!context || box.width === 0 || box.height === 0) {
    const frame = requestAnimationFrame(onDone);
    return () => cancelAnimationFrame(frame);
  }
  const ratio = Math.min(window.devicePixelRatio || 1, MAX_PIXEL_RATIO);
  canvas.width = Math.round(box.width * ratio);
  canvas.height = Math.round(box.height * ratio);
  // The same crop as the <img> under it, so each dot shows the pixels the photo will have there.
  const picture = coverRect(box, PHOTO_SIZE, FRAME_CROP);
  const dots = waveDots(box, pointIn(picture, PHOTO_SIZE, lamp));
  const startedAt = performance.now();
  let frame = 0;

  const draw = (now: number) => {
    const t = (now - startedAt) / WAVE_MS;
    if (t >= 1 || !isOnScreen()) {
      onDone();
      return;
    }
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.globalCompositeOperation = "source-over";
    context.clearRect(0, 0, box.width, box.height);
    context.beginPath();
    for (const dot of dots) {
      const radius = dotRadius(t, dot.start);
      if (radius <= 0) break; // sorted by start: no later dot has started yet
      context.moveTo(dot.x + radius, dot.y);
      context.arc(dot.x, dot.y, radius, 0, 2 * Math.PI);
    }
    context.fill();
    context.globalCompositeOperation = "source-in";
    context.drawImage(
      next,
      picture.x,
      picture.y,
      picture.width,
      picture.height,
    );
    frame = requestAnimationFrame(draw);
  };
  frame = requestAnimationFrame(draw);

  return () => {
    cancelAnimationFrame(frame);
    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
  };
}
