// Package queue holds the rules of a session's queue as one pure function. Step takes the state
// of a session and one action and returns the state after it, or an error when the rules forbid
// the action (FR-QUEUE-1..3, FR-QUEUE-8..9, FR-HAND-2, FR-HAND-4, FR-HAND-6..8, FR-SESS-4,
// FR-SESS-6). It reads no clock and no database: the store loads the state inside a transaction,
// calls Step and writes back what changed, so two requests at the same moment cannot both win.
//
// This file is the contract between the rules, the store and the handlers. It is frozen while
// the parallel build of Phase 2 runs (notes/phase-2-plan.md): change it only through the plan.
package queue

import (
	"errors"
	"fmt"
	"time"
)

// Status is where a hand is in its life (FR-QUEUE-2). The same words are in the database.
type Status string

const (
	Waiting    Status = "waiting"
	Answering  Status = "answering"
	Answered   Status = "answered"
	Skipped    Status = "skipped"
	Withdrawn  Status = "withdrawn"
	Removed    Status = "removed"
	NotReached Status = "not_reached"
	Carried    Status = "carried"
	Merged     Status = "merged"
)

// IsOpen reports whether a hand with this status is still in the queue.
func (s Status) IsOpen() bool { return s == Waiting || s == Answering }

// Hand is one question, as the rules see it.
type Hand struct {
	ID         int64 // 0 for a hand that Step has just raised: the store gives it its id
	AuthorID   string
	Title      string
	Details    string
	Status     Status
	RaisedAt   time.Time // never changes
	SortAt     time.Time // the queue's order, oldest first; ties go to the smaller ID
	TakenAt    time.Time // when it became Answering; zero if it never did
	TakenBy    string    // the maintainer who pressed Next
	ClosedAt   time.Time // when it left the queue; zero while open
	AckedAt    time.Time // when its author pressed "I'm here" (FR-QUEUE-9); zero if not
	AwayCount  int       // how often its author missed their turn (FR-QUEUE-8)
	MergedInto int64     // the hand it was merged into (FR-HAND-8), or 0
	Plus       []string  // the user ids who pressed "+1 me too" (FR-HAND-7), in that order
}

// State is one session with every hand of it that is still open, in no particular order. What
// Step returns holds the same hands: one that the action closed is still there, with its new
// status, and a hand that was just raised is added with ID 0.
type State struct {
	Live        bool
	HandsClosed bool // last call: no new hands (FR-SESS-6)
	Hands       []Hand
}

// Kind names what someone wants to do.
type Kind string

const (
	Raise    Kind = "raise"    // Title, Details. The actor becomes the author.
	Withdraw Kind = "withdraw" // HandID. Only its author, only while it waits (FR-HAND-4).
	Plus     Kind = "plus"     // HandID. Adds the actor's +1 to a waiting hand, or takes it back.
	Here     Kind = "here"     // HandID. The author of the hand being answered says "I'm here".
	Next     Kind = "next"     // The actor takes the first waiting hand (FR-QUEUE-3).
	Skip     Kind = "skip"     // HandID, waiting or being answered.
	Remove   Kind = "remove"   // HandID, waiting or being answered (FR-MOD-1).
	Away     Kind = "away"     // HandID, the hand being answered whose author is not there (FR-QUEUE-8).
	Merge    Kind = "merge"    // HandID, a waiting hand, into the hand being answered (FR-HAND-8).
	Done     Kind = "done"     // HandID, the hand being answered.
	LastCall Kind = "lastcall" // Closed: close hands, or open them again (FR-SESS-6).
	End      Kind = "end"      // Carry: waiting hands go to the next session, or are not reached (FR-SESS-4).
)

// Action is one thing someone does to a session's queue. Whether the actor may do it by their
// role (member, maintainer) is the guard's question, asked before Step. Step checks what only
// the hands can tell: whose a hand is, and what state it is in.
type Action struct {
	Kind    Kind
	ActorID string
	At      time.Time // now, given by the caller so that Step stays pure
	HandID  int64
	Title   string
	Details string
	Closed  bool // LastCall
	Carry   bool // End
	// LastRaisedAt is when the actor last raised a hand in this project, in any session, or zero
	// (FR-HAND-6). The store fills it in for a Raise.
	LastRaisedAt time.Time
}

// RaiseEvery is how long someone waits between two hands in one project (FR-HAND-6).
const RaiseEvery = 60 * time.Second

// What Step refuses. The handlers turn each into a status and a sentence (queueRefusal in
// server/api_queue.go), so compare with errors.Is and errors.As, never by text.
var (
	ErrNotImplemented = errors.New("queue: this action is not built yet")
	ErrNotLive        = errors.New("queue: the session is not live")
	ErrHandsClosed    = errors.New("queue: hands are closed")                      // FR-SESS-6
	ErrAlreadyOpen    = errors.New("queue: the author already has an open hand")   // FR-HAND-2
	ErrNoSuchHand     = errors.New("queue: no open hand with this id")             //
	ErrNotYours       = errors.New("queue: only the hand's author can do this")    // FR-HAND-4, FR-QUEUE-9
	ErrOwnHand        = errors.New("queue: a +1 on one's own hand")                // FR-HAND-7
	ErrNothingWaiting = errors.New("queue: nobody is waiting")                     // FR-QUEUE-3
	ErrAnswering      = errors.New("queue: a hand is being answered")              // Next and End wait for it
	ErrNotAnswering   = errors.New("queue: this hand is not being answered")       // Here, Away, Done, Merge
	ErrIsHere         = errors.New("queue: the author said they are here")         // Away (FR-QUEUE-9)
	ErrBadStatus      = errors.New("queue: the hand's status does not allow this") // FR-QUEUE-2
)

// TooSoonError refuses a hand raised less than RaiseEvery after the author's last one (FR-HAND-6).
type TooSoonError struct {
	Wait time.Duration // how much longer to wait
}

func (e *TooSoonError) Error() string {
	return fmt.Sprintf("queue: raised too soon, wait %s", e.Wait.Round(time.Second))
}
