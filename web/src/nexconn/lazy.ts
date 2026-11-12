// The Nexconn SDK and Chat UI are about 1.4 MB. They load only once someone is signed in, and
// never on the landing page.
let loading: Promise<typeof import("./client")> | null = null;

export function loadClient(): Promise<typeof import("./client")> {
  loading ??= import("./client");
  return loading;
}

/** The code that opens a room in Chat UI. It shares the SDK's chunk. */
export function loadRoom(): Promise<typeof import("./room")> {
  return import("./room");
}
