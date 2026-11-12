/** A refusal or failure of an API call, with a sentence that can be shown as it is. */
export class ApiError extends Error {
  readonly status: number; // 0 when the server could not be reached

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
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

async function request<T>(
  method: "GET" | "POST" | "DELETE",
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit =
    method === "GET"
      ? { method }
      : {
          method,
          // Every mutation is JSON, even an empty one: the server refuses anything else (NFR-SEC-5).
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body ?? {}),
        };

  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    throw new ApiError(
      0,
      "Can't reach the server. Check your connection and try again.",
    );
  }

  // A proxy or platform error page is HTML. It must not reach the user as a parse error.
  const payload: unknown = await res.json().catch(() => null);
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
  delete: <T = void>(path: string, body?: unknown) =>
    request<T>("DELETE", path, body),
};
