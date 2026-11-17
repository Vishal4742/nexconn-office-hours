/**
 * Wraps a load so that the newest answer to arrive wins: an answer is used unless the answer to a
 * later call was used already. A later call that fails takes nothing away, since the older answer
 * is still news (state-machine audit, bug 13). The returned call rejects when its load did.
 */
export function newestAnswer<T>(
  load: () => Promise<T>,
  use: (value: T) => void,
): () => Promise<void> {
  let calls = 0;
  let used = 0;
  return async () => {
    const call = ++calls;
    const value = await load();
    if (call < used) return;
    used = call;
    use(value);
  };
}
