// The Nexconn SDK and Chat UI are about 1.4 MB. They load only once someone is signed in, and
// never on the landing page. The browser keeps a module it has loaded and asks again for one that
// failed, so nothing is remembered here.
export function loadClient(): Promise<typeof import("./client")> {
  return import("./client");
}

/** The code that opens a room in Chat UI: a small file of its own, loaded with the first room. */
export function loadRoom(): Promise<typeof import("./room")> {
  return import("./room");
}
