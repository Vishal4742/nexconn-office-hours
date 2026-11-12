// Queues in every state the room can show, for tests and for looking at a part of the room
// without running a session. This file is a contract (notes/phase-2-plan.md): add nothing here
// from a worker's branch; build what you need from these in your own test.
import type { Hand, Queue, QueuePerson, QueueSession } from "./types";

export const mia: QueuePerson = {
  userId: "gh1",
  login: "mia-k",
  name: "Mia Kowalski",
  avatarUrl: "",
};
export const diego: QueuePerson = {
  userId: "gh2",
  login: "diego-ruiz",
  name: "Diego Ruiz",
  avatarUrl: "",
};
export const ana: QueuePerson = {
  userId: "gh3",
  login: "ana-ferreira",
  name: "Ana Ferreira",
  avatarUrl: "",
};
export const liwen: QueuePerson = {
  userId: "gh4",
  login: "liwen",
  name: "Li Wen",
  avatarUrl: "",
};

/** The server's clock in every fixture: 20 minutes into a 45 minute session. */
export const NOW = "2026-11-14T11:50:00.000Z";

export const liveSession: QueueSession = {
  id: "12",
  number: 3,
  title: "Thursday office hours",
  live: true,
  handsClosed: false,
  startedAt: "2026-11-14T11:30:00.000Z",
  endedAt: null,
  plannedMinutes: 45,
  version: 9,
  host: mia,
};

/** A waiting hand with nothing special about it; give what a test is about. */
export function hand(
  id: string,
  author: QueuePerson,
  over: Partial<Hand> = {},
): Hand {
  return {
    id,
    title: "How do I retry only on 5xx?",
    details: "",
    status: "waiting",
    author,
    raisedAt: "2026-11-14T11:35:00.000Z",
    takenAt: null,
    takenBy: null,
    ackedAt: null,
    closedAt: null,
    awayCount: 0,
    plusCount: 0,
    plusByMe: false,
    isMine: false,
    faqId: "",
    faqSlug: "",
    ...over,
  };
}

const empty: Queue = {
  session: null,
  hands: [],
  done: [],
  nextSessionAt: null,
  medianAnswerSeconds: 0,
  serverTime: NOW,
};

const answered = hand("100", liwen, {
  title: "Where are the docs for custom cache keys?",
  status: "answered",
  takenAt: "2026-11-14T11:33:00.000Z",
  takenBy: mia,
  closedAt: "2026-11-14T11:38:00.000Z",
  faqId: "7",
  faqSlug: "where-are-the-docs-for-custom-cache-keys",
});
const beingAnswered = hand("101", diego, {
  status: "answering",
  details: "The docs show retry: 3, but that also retries a 404.",
  takenAt: "2026-11-14T11:44:00.000Z",
  takenBy: mia,
  plusCount: 1,
});
const waiting = [
  hand("102", ana, {
    title: "Can the cache be shared between tabs?",
    raisedAt: "2026-11-14T11:40:00.000Z",
    plusCount: 2,
  }),
  hand("103", liwen, {
    title: "Is there a way to revalidate on focus?",
    raisedAt: "2026-11-14T11:41:00.000Z",
  }),
];

/** The room as each kind of viewer meets it. Diego is "me" where a hand is mine. */
export const queues = {
  /** The project never had a session. */
  noSession: empty,
  /** No session now, and the next one is planned. */
  nextPlanned: { ...empty, nextSessionAt: "2026-11-19T11:30:00.000Z" },
  /** Live, nobody has raised a hand yet. */
  liveEmpty: { ...empty, session: liveSession },
  /** Live, as someone without a hand: one being answered, two waiting, one done. */
  liveWatching: {
    ...empty,
    session: liveSession,
    hands: [beingAnswered, ...waiting],
    done: [answered],
    medianAnswerSeconds: 300,
  },
  /** Live, my hand is third in line: two ahead of me. */
  liveWaiting: {
    ...empty,
    session: liveSession,
    hands: [
      hand("101", ana, { status: "answering", takenBy: mia }),
      ...waiting,
      hand("104", diego, {
        isMine: true,
        raisedAt: "2026-11-14T11:42:00.000Z",
      }),
    ],
    done: [answered],
    medianAnswerSeconds: 300,
  },
  /** Live, my hand is the first that waits: "You're next". */
  liveNext: {
    ...empty,
    session: liveSession,
    hands: [
      hand("101", ana, { status: "answering", takenBy: mia }),
      hand("104", diego, { isMine: true }),
      ...waiting,
    ],
  },
  /** Live, it is my turn and I have not said "I'm here" yet. */
  myTurn: {
    ...empty,
    session: liveSession,
    hands: [{ ...beingAnswered, isMine: true }, ...waiting],
  },
  /** Last call: hands are closed, two are left. */
  lastCall: {
    ...empty,
    session: { ...liveSession, handsClosed: true },
    hands: waiting,
    done: [answered],
  },
  /** The session ended: one answered, one carried over, one not reached, one removed. */
  ended: {
    ...empty,
    session: {
      ...liveSession,
      live: false,
      endedAt: "2026-11-14T12:15:00.000Z",
      version: 31,
    },
    done: [
      hand("102", ana, { status: "carried" }),
      hand("103", liwen, { status: "not_reached" }),
      hand("105", diego, { status: "removed", title: "", details: "" }),
      answered,
    ],
    nextSessionAt: "2026-11-19T11:30:00.000Z",
  },
} as const satisfies Record<string, Queue>;
