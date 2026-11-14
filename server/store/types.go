package store

import (
	"errors"
	"time"

	"officehours/queue"
)

// The shapes the queue, the FAQ and the stats pass between the store and the handlers. This file
// is a contract: it is frozen while the parallel build of Phase 2 runs (notes/phase-2-plan.md).

// ErrSessionLive refuses a second live session in one project (FR-SESS-2).
var ErrSessionLive = errors.New("store: the project already has a live session")

// Person is someone as the queue and the FAQ show them. The login is how a hand names its author.
type Person struct {
	UserID    string
	Login     string
	Name      string // never empty
	AvatarURL string
}

// Session is one session of a project.
type Session struct {
	ID             int64
	ProjectID      string
	Number         int // 1 for the project's first session
	Title          string
	Live           bool
	PlannedMinutes int // 0 when no length was planned
	StartedAt      time.Time
	EndedAt        time.Time // zero while live
	HandsClosed    bool      // last call (FR-SESS-6)
	Version        int       // goes up by one with every change of the queue (FR-QUEUE-4)
	Host           Person    // who started it
}

// NewSession is what a maintainer gives to start one (FR-SESS-1).
type NewSession struct {
	ProjectID      string
	HostID         string
	Title          string
	PlannedMinutes int // 0 when no length is planned
}

// QueueHand is a hand with the people around it.
type QueueHand struct {
	queue.Hand
	Author  Person
	Taker   Person // the maintainer who took it; zero if nobody did
	FAQID   int64  // the FAQ entry of an answered hand (FR-FAQ-1); 0 if it has none or it was deleted
	FAQSlug string // that entry's slug; "" with FAQID 0
}

// Snapshot is everything the room shows about a project's queue (FR-QUEUE-4). Nothing in it
// depends on who is looking: the handler adds "is this mine" for the caller.
type Snapshot struct {
	Session       *Session      // the live session, else the last one that ended, else nil
	Open          []QueueHand   // the hand being answered first, then the waiting ones in queue order
	Closed        []QueueHand   // every other hand of the session, the latest first
	NextSessionAt time.Time     // FR-SESS-5; zero when not set
	MedianAnswer  time.Duration // of this session's answered hands; zero until there are two (FR-QUEUE-10)
	Removed       []int64       // hands a maintainer removed in any session of the project, newest first (FR-MOD-1)
}

// Applied is what one action did to a session's queue.
type Applied struct {
	ProjectID string
	SessionID int64
	Version   int // the queue's version after the action
	Before    queue.State
	After     queue.State // a hand that was just raised has its id here
}

// FAQDraft is the entry that a Done creates (FR-FAQ-1). The store makes the slug unique in the
// project by adding "-2", "-3" and so on, and keeps the whole within 80 characters, so a draft's
// slug is at most 72.
type FAQDraft struct {
	Slug     string
	Title    string
	Question string
	Answer   string
	Links    []string
	AuthorID string // the maintainer who answered
}

// FAQEntry is one public answer.
type FAQEntry struct {
	ID        int64
	ProjectID string
	HandID    int64 // 0 if the hand is gone
	Slug      string
	Title     string
	Question  string
	Answer    string
	Links     []string
	Author    Person // the maintainer who answered
	AskedBy   int    // how many people asked: the public page shows the number only (FR-HAND-8)
	CreatedAt time.Time
	Deleted   bool // its page answers 410 (FR-FAQ-6)
}

// Stats are the five numbers of FR-MET-1.
type Stats struct {
	Sessions      int
	HandsRaised   int
	HandsAnswered int
	FAQEntries    int
	Participants  int // people who joined a room
}
