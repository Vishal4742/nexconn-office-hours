/**
 * NFR-COMP-1: Chat UI supports Chrome, Edge and Safari; Firefox gets a note. Firefox on iOS runs
 * on Safari's engine and needs none.
 */
export function isFirefox(userAgent: string): boolean {
  return /\bFirefox\//.test(userAgent) && !/\bFxiOS\//.test(userAgent);
}
