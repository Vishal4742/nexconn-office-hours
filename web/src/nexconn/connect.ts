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
