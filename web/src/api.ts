/** A refusal or failure of an API call, with a sentence that can be shown as it is. */
export class ApiError extends Error {
  readonly status: number; // 0 when the server could not be reached

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** The sentence to show for anything a call threw. */
export function errorMessage(failure: unknown): string {
  return failure instanceof Error
    ? failure.message
    : "Something went wrong. Try again.";
}

/** A failed attempt at something, as a form shows it. */
export type Refusal = {
  readonly message: string;
  /** The session ran out, or GitHub no longer accepts its token: signing in again is the cure. */
  readonly needsSignIn: boolean;
};

/** What a form that sends one request is doing. */
export type Sending =
  | { readonly status: "idle" }
  | { readonly status: "sending" }
  | ({ readonly status: "refused" } & Refusal);

export function refusalOf(failure: unknown): Refusal {
  return {
    message: errorMessage(failure),
    needsSignIn: failure instanceof ApiError && failure.status === 401,
  };
}

type Envelope = {
  readonly ok: boolean;
  readonly data?: unknown;
  readonly error?: string;
};

function isEnvelope(body: unknown): body is Envelope {
  return (
    typeof body === "object" &&
    body !== null &&
    "ok" in body &&
    typeof body.ok === "boolean"
  );
}

function fallbackMessage(status: number): string {
  return status >= 500
    ? `The server had a problem (${status}). Try again in a moment.`
    : `The server refused the request (${status}).`;
}

/**
 * How long a call may take, body included, before the page gives up on it. A fetch stuck on a
 * dead connection (a laptop that woke up) would otherwise hold up every later queue fetch
 * (state-machine audit, bug 11). The server answers every request within 25 s (requestDeadline
 * in server/main.go), so a call still open after this went nowhere.
 */
export const REQUEST_TIMEOUT_MS = 30_000;

const tooSlow = () =>
  new ApiError(0, "The server took too long to answer. Try again.");

async function request<T>(
  method: "GET" | "POST",
  path: string,
  body?: unknown,
): Promise<T> {
  const timeout = new AbortController();
  const timer = setTimeout(() => timeout.abort(), REQUEST_TIMEOUT_MS);
  try {
    return await send<T>(method, path, body, timeout.signal);
  } finally {
    clearTimeout(timer);
  }
}

async function send<T>(
  method: "GET" | "POST",
  path: string,
  body: unknown,
  signal: AbortSignal,
): Promise<T> {
  const init: RequestInit =
    method === "GET"
      ? { method, signal }
      : {
          method,
          signal,
          // Every mutation is JSON, even an empty one: the server refuses anything else (NFR-SEC-5).
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body ?? {}),
        };

  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    if (signal.aborted) throw tooSlow();
    throw new ApiError(
      0,
      "Can't reach the server. Check your connection and try again.",
    );
  }

  // A proxy or platform error page is HTML. It must not reach the user as a parse error.
  const payload: unknown = await res.json().catch(() => null);
  if (signal.aborted) throw tooSlow(); // the body stopped arriving
  if (isEnvelope(payload)) {
    if (res.ok && payload.ok) return payload.data as T;
    throw new ApiError(
      res.status,
      payload.error || fallbackMessage(res.status),
    );
  }
  throw new ApiError(res.status, fallbackMessage(res.status));
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T = void>(path: string, body?: unknown) =>
    request<T>("POST", path, body),
};
