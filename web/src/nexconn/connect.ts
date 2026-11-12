/** How a connection attempt ended. `code` is the Nexconn SDK's error code. */
export type ConnectOutcome =
  { readonly ok: true } | { readonly ok: false; readonly code: number };

// The two codes that mean "this token is no good": RC_CONN_TOKEN_INCORRECT and
// RC_CONN_TOKEN_EXPIRED in the SDK's ErrorCode list.
const TOKEN_REFUSED: readonly number[] = [31004, 31020];

type Deps = {
  /** Asks our server for the caller's Nexconn token. */
  readonly fetchToken: () => Promise<string>;
  readonly connect: (token: string) => Promise<ConnectOutcome>;
};

/**
 * FR-AUTH-3: connect with a token from our server; if Nexconn says the token is invalid, fetch a
 * new one once and try again. Any other failure is returned as it is: the SDK reconnects on its
 * own after network trouble.
 */
export async function connectWithFreshToken(
  deps: Deps,
): Promise<ConnectOutcome> {
  const first = await deps.connect(await deps.fetchToken());
  if (first.ok || !TOKEN_REFUSED.includes(first.code)) return first;
  return deps.connect(await deps.fetchToken());
}

/** Chat UI answers this while its channel list is still loading after a connect. */
export const CHANNEL_LIST_NOT_READY = 39005;

export type OpenOutcome = { readonly isOk: boolean; readonly code: number };

/**
 * Opens a channel, and tries again for as long as Chat UI says its channel list is not ready.
 * Any other refusal is returned at once. `pause` waits between two tries.
 */
export async function openWhenReady<T extends OpenOutcome>(
  open: () => Promise<T>,
  pause: () => Promise<void>,
  attempts = 20,
): Promise<T> {
  let outcome = await open();
  for (let tries = 1; tries < attempts; tries++) {
    if (outcome.isOk || outcome.code !== CHANNEL_LIST_NOT_READY) break;
    await pause();
    outcome = await open();
  }
  return outcome;
}
