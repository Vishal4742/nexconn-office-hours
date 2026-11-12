package queue

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// awaySkips is the miss that costs a hand its turn for good: the first sends it two places back,
// the second to the end, the third skips it (FR-QUEUE-8).
const awaySkips = 3

// Step is the one rule function: the state after the action, or why the rules refuse it. It
// follows the table in §4 of notes/phase-2-plan.md row by row, refusals in the order given there.
//
// Step is pure. It reads no clock (the action carries its time) and never changes its argument:
// the result has a list of hands of its own, and a +1 list is copied before it changes. The
// result holds the same hands as the argument. One that the action closed is still there, with
// its new status and ClosedAt; a raised hand is added with ID 0. A refusal comes with the zero
// State.
//
// Only open hands count. A closed hand that is still in the list (Step's own result, stepped
// again) is carried along as it is, and an action that names it gets ErrNoSuchHand, as it does
// when the store has left the hand out.
func Step(s State, a Action) (State, error) {
	if !s.Live {
		return State{}, ErrNotLive
	}
	s.Hands = slices.Clone(s.Hands) // s is our own from here on: the caller's list is never written
	var err error
	switch a.Kind {
	case Raise:
		err = raise(&s, a)
	case Next:
		err = next(s.Hands, a)
	case LastCall:
		s.HandsClosed = a.Closed
	case End:
		err = end(&s, a)
	case Withdraw, Plus, Here, Skip, Remove, Away, Merge, Done:
		err = onHand(s.Hands, a)
	default:
		err = ErrNotImplemented
	}
	if err != nil {
		return State{}, err
	}
	return s, nil
}

// raise adds the actor's hand at the back of the line (FR-HAND-2, FR-HAND-6, FR-SESS-6).
func raise(s *State, a Action) error {
	hasOpen := func(h Hand) bool { return h.AuthorID == a.ActorID && h.Status.IsOpen() }
	since := a.At.Sub(a.LastRaisedAt)
	switch {
	case s.HandsClosed:
		return ErrHandsClosed
	case slices.ContainsFunc(s.Hands, hasOpen):
		return ErrAlreadyOpen
	case !a.LastRaisedAt.IsZero() && since < RaiseEvery:
		return &TooSoonError{Wait: RaiseEvery - since}
	}
	s.Hands = append(s.Hands, Hand{
		AuthorID: a.ActorID, Title: a.Title, Details: a.Details,
		Status: Waiting, RaisedAt: a.At, SortAt: a.At,
	})
	return nil
}

// next gives the first in line their turn (FR-QUEUE-3).
func next(hands []Hand, a Action) error {
	line := waiting(hands)
	switch {
	case answering(hands) != nil:
		return ErrAnswering
	case len(line) == 0:
		return ErrNothingWaiting
	}
	h := line[0]
	h.Status, h.TakenAt, h.TakenBy, h.AckedAt = Answering, a.At, a.ActorID, time.Time{}
	return nil
}

// end closes the session: whoever still waits is carried to the next one, or was not reached
// (FR-SESS-4). A hand being answered has no way there (FR-QUEUE-2), so it is finished first.
func end(s *State, a Action) error {
	if answering(s.Hands) != nil {
		return ErrAnswering
	}
	as := NotReached
	if a.Carry {
		as = Carried
	}
	for _, h := range waiting(s.Hands) {
		shut(h, as, a.At)
	}
	s.Live = false
	return nil
}

// onHand does what an action does to the open hand it names.
func onHand(hands []Hand, a Action) error {
	i := slices.IndexFunc(hands, func(h Hand) bool { return h.ID == a.HandID && h.Status.IsOpen() })
	if i < 0 {
		return ErrNoSuchHand
	}
	h := &hands[i]
	switch a.Kind {
	case Withdraw:
		return withdraw(h, a)
	case Plus:
		return plus(h, a)
	case Here:
		return here(h, a)
	case Away:
		return away(hands, h, a)
	case Merge:
		return merge(hands, h, a)
	case Done:
		if h.Status != Answering {
			return ErrNotAnswering
		}
		shut(h, Answered, a.At)
	case Skip:
		shut(h, Skipped, a.At)
	case Remove:
		shut(h, Removed, a.At)
	}
	return nil
}

// withdraw takes a waiting hand back; only its author can (FR-HAND-4).
func withdraw(h *Hand, a Action) error {
	switch {
	case h.AuthorID != a.ActorID:
		return ErrNotYours
	case h.Status != Waiting:
		return ErrBadStatus
	}
	shut(h, Withdrawn, a.At)
	return nil
}

// plus adds the actor's "+1 me too" to someone else's waiting hand, or takes it back
// (FR-HAND-7). The hand's place in the line never changes.
func plus(h *Hand, a Action) error {
	switch {
	case h.AuthorID == a.ActorID:
		return ErrOwnHand
	case h.Status != Waiting:
		return ErrBadStatus
	}
	if slices.Contains(h.Plus, a.ActorID) {
		h.Plus = plusSet(h.Plus, a.ActorID)
	} else {
		h.Plus = plusSet(slices.Concat(h.Plus, []string{a.ActorID}), h.AuthorID)
	}
	return nil
}

// here records that the author of the hand being answered said "I'm here" (FR-QUEUE-9). A second
// press keeps the first time.
func here(h *Hand, a Action) error {
	switch {
	case h.AuthorID != a.ActorID:
		return ErrNotYours
	case h.Status != Answering:
		return ErrNotAnswering
	}
	if h.AckedAt.IsZero() {
		h.AckedAt = a.At
	}
	return nil
}

// away is for the hand being answered whose author is not there (FR-QUEUE-8). The first miss
// sends it two places back, the second to the end, the third skips it.
func away(hands []Hand, h *Hand, a Action) error {
	switch {
	case h.Status != Answering:
		return ErrNotAnswering
	case !h.AckedAt.IsZero():
		return ErrIsHere
	}
	h.AwayCount++
	if h.AwayCount >= awaySkips {
		shut(h, Skipped, a.At)
		return nil
	}
	h.SortAt = awayPlace(waiting(hands), h.AwayCount, a.At) // the line without h, which does not wait yet
	h.Status, h.TakenAt, h.TakenBy, h.AckedAt = Waiting, time.Time{}, "", time.Time{}
	return nil
}

// awayPlace is the SortAt of a hand that goes back into the line after a miss; line is who waits
// already. Two places back is halfway between the second and the third of them, cut to the whole
// microsecond, so it is never as late as the third. On a second miss, with fewer than three
// waiting, or when the cut lands on the second because no microsecond lies between the two, the
// hand goes to the end: later than everyone who waits, and not earlier than now.
func awayPlace(line []*Hand, misses int, now time.Time) time.Time {
	if misses == 1 && len(line) >= 3 {
		second, third := line[1].SortAt, line[2].SortAt
		if mid := second.Add(third.Sub(second) / 2).Truncate(time.Microsecond); mid.After(second) {
			return mid
		}
	}
	if n := len(line); n > 0 && !line[n-1].SortAt.Before(now) {
		return line[n-1].SortAt.Add(time.Microsecond)
	}
	return now
}

// merge joins a waiting hand to the one being answered: its author and its +1s asked that
// question too (FR-HAND-8). The +1s are copied; the merged hand keeps its own list.
func merge(hands []Hand, h *Hand, a Action) error {
	into := answering(hands)
	switch {
	case h.Status != Waiting:
		return ErrBadStatus
	case into == nil:
		return ErrNotAnswering
	}
	into.Plus = plusSet(slices.Concat(into.Plus, h.Plus, []string{h.AuthorID}), into.AuthorID)
	h.MergedInto = into.ID
	shut(h, Merged, a.At)
	return nil
}

// shut closes a hand: it leaves the queue with the status given.
func shut(h *Hand, as Status, at time.Time) {
	h.Status, h.ClosedAt = as, at
}

// answering is the hand being answered, or nil when it is nobody's turn.
func answering(hands []Hand) *Hand {
	i := slices.IndexFunc(hands, func(h Hand) bool { return h.Status == Answering })
	if i < 0 {
		return nil
	}
	return &hands[i]
}

// waiting is the line: the hands that wait, in the queue's order. That is by SortAt, then by
// ID, with a hand that has no id yet (ID 0, just raised) behind its equals, because the store
// will give it the highest.
func waiting(hands []Hand) []*Hand {
	var line []*Hand
	for i := range hands {
		if hands[i].Status == Waiting {
			line = append(line, &hands[i])
		}
	}
	rank := func(id int64) int64 {
		if id == 0 {
			return math.MaxInt64
		}
		return id
	}
	slices.SortStableFunc(line, func(a, b *Hand) int {
		return cmp.Or(a.SortAt.Compare(b.SortAt), cmp.Compare(rank(a.ID), rank(b.ID)))
	})
	return line
}

// plusSet is a +1 list as the rules keep it: a new list, sorted by user id, every id once,
// without the one given, and nil when nobody is left. The list it is given is never changed.
func plusSet(ids []string, without string) []string {
	var set []string
	for _, id := range ids {
		if id != without {
			set = append(set, id)
		}
	}
	slices.Sort(set)
	return slices.Compact(set)
}
