import { useEffect, useState } from "react";

import { serverNow, subscribeQueue } from "../../queue/store";

/**
 * The server's clock (plan decision 12), read again every `everyMs` and whenever the queue
 * changes: the first answer of the server is what sets the clock, and a page that waited for the
 * next tick would count minutes on its own clock until then.
 */
export function useServerNow(everyMs: number): number {
  const [now, setNow] = useState(serverNow);
  useEffect(() => {
    const read = () => setNow(serverNow());
    const timer = setInterval(read, everyMs);
    const stop = subscribeQueue(read);
    return () => {
      clearInterval(timer);
      stop();
    };
  }, [everyMs]);
  return now;
}
