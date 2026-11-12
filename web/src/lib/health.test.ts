import { describe, expect, it } from "vitest";

import { isHealthy } from "./health";

describe("isHealthy", () => {
  it.each([
    { name: "the server's ok envelope", body: { ok: true }, want: true },
    {
      name: "an error envelope",
      body: { ok: false, error: "down" },
      want: false,
    },
    { name: "ok that is not a boolean", body: { ok: "true" }, want: false },
    {
      name: "a proxy error page read as text",
      body: "<html>502</html>",
      want: false,
    },
    { name: "null", body: null, want: false },
    { name: "an array", body: [true], want: false },
  ])("$name -> $want", ({ body, want }) => {
    expect(isHealthy(body)).toBe(want);
  });
});
