/**
 * Wraps a task so that one run is on its way at a time (plan decision 4). A call while it runs
 * asks for exactly one more run afterwards, however many calls there were.
 */
export function oneAtATime(task: () => Promise<void>): () => void {
  let isRunning = false;
  let isAskedAgain = false;

  const run = (): void => {
    isRunning = true;
    isAskedAgain = false;
    // The task handles its own failures. This only makes sure that a broken run, even one that
    // throws before it returns a promise, never blocks the next one.
    new Promise<void>((resolve) => resolve(task()))
      .catch(() => undefined)
      .then(() => {
        isRunning = false;
        if (isAskedAgain) run();
      });
  };

  return () => {
    if (isRunning) isAskedAgain = true;
    else run();
  };
}
