import { describe, expect, it } from "vitest";

import { calendarPath, googleCalendarUrl, subscribeUrl } from "./calendar";

const repo = { owner: "PaperPlane", repo: "Stash" };

describe("calendar links", () => {
  it("FR-SESS-5: Google Calendar opens with the session filled in, in UTC", () => {
    const url = new URL(
      googleCalendarUrl(
        repo,
        "2026-11-19T11:30:00.123456Z",
        45,
        "https://office-hours-oss.vercel.app",
      ),
    );
    expect(url.origin + url.pathname).toBe(
      "https://calendar.google.com/calendar/render",
    );
    expect(url.searchParams.get("action")).toBe("TEMPLATE");
    expect(url.searchParams.get("text")).toBe("Office hours: PaperPlane/Stash");
    expect(url.searchParams.get("dates")).toBe(
      "20261119T113000Z/20261119T121500Z",
    );
    expect(url.searchParams.get("location")).toBe(
      "https://office-hours-oss.vercel.app/p/PaperPlane/Stash",
    );
  });

  it("takes 45 minutes when no length is planned, and nothing for a time that can't be read", () => {
    const dates = new URL(
      googleCalendarUrl(repo, "2026-11-19T11:30:00Z", 0, "https://x.example"),
    ).searchParams.get("dates");
    expect(dates).toBe("20261119T113000Z/20261119T121500Z");
    expect(googleCalendarUrl(repo, "soon", 45, "https://x.example")).toBe("");
  });

  it("the feed is on our own origin; subscribing uses webcal://", () => {
    expect(calendarPath(repo)).toBe("/p/PaperPlane/Stash/calendar.ics");
    expect(subscribeUrl(repo, "https://office-hours-oss.vercel.app")).toBe(
      "webcal://office-hours-oss.vercel.app/p/PaperPlane/Stash/calendar.ics",
    );
    expect(subscribeUrl(repo, "http://localhost:5173")).toBe(
      "webcal://localhost:5173/p/PaperPlane/Stash/calendar.ics",
    );
  });
});
