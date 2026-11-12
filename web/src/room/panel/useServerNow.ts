import { useEffect, useState } from "react";

import { serverNow } from "../../queue/store";

/** The server's clock (plan decision 12), read again every `everyMs`. */
export function useServerNow(everyMs: number): number {
  const [now, setNow] = useState(serverNow);
  useEffect(() => {
    const timer = setInterval(() => setNow(serverNow()), everyMs);
    return () => clearInterval(timer);
  }, [everyMs]);
  return now;
}
