/** True only for the server's `{ ok: true }` envelope. The body is untrusted until narrowed. */
export function isHealthy(body: unknown): boolean {
  return (
    typeof body === "object" &&
    body !== null &&
    "ok" in body &&
    body.ok === true
  );
}
