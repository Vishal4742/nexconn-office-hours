package queue

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// Whole sessions: many actions one after the other, each stepping the state the one before it
// left, the way the store will. The acceptance tests named here are run by hand on production;
// these are their rules.

// desk plays the store around Step. It keeps the state, gives a raised hand its id, remembers
// when each person last raised a hand (Action.LastRaisedAt), and after every action that Step
// allowed it checks what must hold in any session (checkStep). Unlike the store it keeps the
// hands that left the queue in the state, as Step returned them.
type desk struct {
	t      *testing.T
	state  State
	lastID int64
	raised map[string]time.Time
}

func newDesk(t *testing.T) *desk {
	t.Helper()
	return &desk{t: t, state: State{Live: true}, raised: map[string]time.Time{}}
}

// do sends one action through Step and keeps the state it returns.
func (d *desk) do(a Action) error {
	d.t.Helper()
	if a.Kind == Raise {
		a.LastRaisedAt = d.raised[a.ActorID]
	}
	after, err := pureStep(d.t, d.state, a)
	if err != nil {
		return err
	}
	checkStep(d.t, d.state, a, after)
	for i := range after.Hands {
		if after.Hands[i].ID == 0 {
			d.lastID++
			after.Hands[i].ID = d.lastID
			d.raised[a.ActorID] = a.At
		}
	}
	d.state = after
	return nil
}

// must is do for an action the rules allow.
func (d *desk) must(a Action) {
	d.t.Helper()
	if err := d.do(a); err != nil {
		d.t.Fatalf("%s by %s on hand %d was refused: %v", a.Kind, a.ActorID, a.HandID, err)
	}
}

// refused is do for an action the rules refuse.
func (d *desk) refused(a Action, want error) {
	d.t.Helper()
	if err := d.do(a); !errors.Is(err, want) {
		d.t.Fatalf("%s by %s on hand %d: got %v, want the refusal %q", a.Kind, a.ActorID, a.HandID, err, want)
	}
}

// raise is a hand raised by someone; it answers the id the hand got.
func (d *desk) raise(who string, at time.Time) int64 {
	d.t.Helper()
	d.must(Action{Kind: Raise, ActorID: who, At: at, Title: "A question from " + who})
	return d.lastID
}

// line fails the test unless exactly these hands wait, in this order.
func (d *desk) line(want ...int64) {
	d.t.Helper()
	if got := lineOf(d.state); !slices.Equal(got, want) {
		d.t.Fatalf("the line is %v, want %v", got, want)
	}
}

// hand is a hand of the session, open or not.
func (d *desk) hand(id int64) Hand {
	d.t.Helper()
	h, ok := handOf(d.state, id)
	if !ok {
		d.t.Fatalf("the session has no hand %d", id)
	}
	return h
}

// current fails the test unless this hand is the one being answered.
func (d *desk) current(want int64) {
	d.t.Helper()
	for _, h := range d.state.Hands {
		if h.Status == Answering && h.ID != want {
			d.t.Fatalf("hand %d is being answered, want hand %d", h.ID, want)
		}
	}
	if got := d.hand(want).Status; got != Answering {
		d.t.Fatalf("hand %d is %s, want it to be the one being answered", want, got)
	}
}

func handOf(s State, id int64) (Hand, bool) {
	i := slices.IndexFunc(s.Hands, func(h Hand) bool { return h.ID == id })
	if i < 0 {
		return Hand{}, false
	}
	return s.Hands[i], true
}

// mayMove reports whether FR-QUEUE-2 lets a hand go from one status to another inside a session.
// What left the queue stays where it is; carried → waiting is the store's, when the next session
// starts.
func mayMove(from, to Status) bool {
	switch from {
	case Waiting:
		return slices.Contains([]Status{Answering, Withdrawn, Skipped, Removed, Merged, NotReached, Carried}, to)
	case Answering:
		return slices.Contains([]Status{Answered, Skipped, Removed, Waiting}, to)
	}
	return false
}

// checkStep fails the test when an action that Step allowed broke what holds in every session:
// no hand is lost or made up (FR-QUEUE-1), a hand stays the question it was, it changes status
// only as FR-QUEUE-2 allows, one hand at most is being answered (FR-QUEUE-3), nobody has two open
// hands (FR-HAND-2), a +1 list is a sorted set without the hand's own author (FR-HAND-7), Next
// takes the first in line (FR-QUEUE-1) and Away puts a hand where FR-QUEUE-8 says.
func checkStep(t *testing.T, before State, a Action, after State) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("after %s by %s on hand %d: %s\nbefore %+v\n after %+v",
			a.Kind, a.ActorID, a.HandID, fmt.Sprintf(format, args...), before, after)
	}
	old := make(map[int64]Hand, len(before.Hands))
	for _, h := range before.Hands {
		old[h.ID] = h
	}
	raised, answered := 0, 0
	open := map[string]bool{}
	for _, h := range after.Hands {
		was, known := old[h.ID]
		delete(old, h.ID)
		switch {
		case !known && h.ID == 0 && a.Kind == Raise:
			raised++
		case !known:
			fail("hand %d was not there before", h.ID)
		case h.AuthorID != was.AuthorID || h.Title != was.Title || h.Details != was.Details || !h.RaisedAt.Equal(was.RaisedAt):
			fail("hand %d is no longer the question it was", h.ID)
		case h.Status != was.Status && !mayMove(was.Status, h.Status):
			fail("FR-QUEUE-2: hand %d went from %s to %s", h.ID, was.Status, h.Status)
		}
		if h.Status == Answering {
			answered++
		}
		if h.Status.IsOpen() && open[h.AuthorID] {
			fail("FR-HAND-2: %s has two open hands", h.AuthorID)
		}
		open[h.AuthorID] = open[h.AuthorID] || h.Status.IsOpen()
		if h.Status.IsOpen() != h.ClosedAt.IsZero() {
			fail("hand %d is %s and was closed at %v", h.ID, h.Status, h.ClosedAt)
		}
		if !slices.IsSorted(h.Plus) || len(slices.Compact(slices.Clone(h.Plus))) != len(h.Plus) || slices.Contains(h.Plus, h.AuthorID) {
			fail("FR-HAND-7: the +1s of hand %d are %v, not a sorted set without its author", h.ID, h.Plus)
		}
	}
	switch {
	case len(old) > 0:
		fail("FR-QUEUE-1: %d hands were lost", len(old))
	case (a.Kind == Raise) != (raised == 1):
		fail("FR-QUEUE-1: %d hands were added", raised)
	case answered > 1:
		fail("FR-QUEUE-3: %d hands are being answered", answered)
	case !after.Live && slices.ContainsFunc(after.Hands, func(h Hand) bool { return h.Status.IsOpen() }):
		fail("FR-SESS-4: the session ended with a hand still open")
	}

	waited := lineOf(before)
	switch a.Kind {
	case Next:
		if len(waited) == 0 {
			fail("FR-QUEUE-3: Next was allowed with nobody waiting")
		}
		if first, _ := handOf(after, waited[0]); first.Status != Answering {
			fail("FR-QUEUE-1: hand %d was first in line and is %s", first.ID, first.Status)
		}
	case Away:
		h, _ := handOf(after, a.HandID)
		if h.Status == Skipped {
			if h.AwayCount != 3 {
				fail("FR-QUEUE-8: hand %d was skipped on miss %d", h.ID, h.AwayCount)
			}
			break
		}
		place := len(waited) // the end
		if h.AwayCount == 1 && len(waited) >= 3 {
			second, _ := handOf(before, waited[1])
			third, _ := handOf(before, waited[2])
			if third.SortAt.Sub(second.SortAt) >= 2*time.Microsecond {
				place = 2 // two places behind: a microsecond lies between the second and the third
			}
		}
		if got := slices.Index(lineOf(after), h.ID); got != place {
			fail("FR-QUEUE-8: after miss %d hand %d stands at place %d of the line, want %d", h.AwayCount, h.ID, got, place)
		}
		if place == len(waited) && h.SortAt.Before(a.At) {
			fail("FR-QUEUE-8: the end of the line is earlier than now")
		}
	}
}

func TestSessions(t *testing.T) {
	pressNext := func(at time.Time) Action { return Action{Kind: Next, ActorID: host, At: at} }

	t.Run("FR-QUEUE-1: first in, first out", func(t *testing.T) {
		d := newDesk(t)
		askers := []string{cyd, ana, eli, ben, dee}
		for i, who := range askers {
			d.raise(who, sec(10*i))
		}
		d.line(1, 2, 3, 4, 5)
		var answered []string
		for i := range askers {
			d.must(pressNext(sec(100 + 60*i)))
			d.current(int64(i + 1))
			d.line([]int64{2, 3, 4, 5}[i:]...)
			answered = append(answered, d.hand(int64(i+1)).AuthorID)
			d.must(act(Done, host, int64(i+1), sec(150+60*i)))
		}
		if !slices.Equal(answered, askers) {
			t.Errorf("answered in the order %v, raised in the order %v", answered, askers)
		}
		d.refused(pressNext(sec(500)), ErrNothingWaiting)
	})

	t.Run("FR-HAND-2, FR-HAND-6: one open hand each and 60 s between two hands (AT-6)", func(t *testing.T) {
		d := newDesk(t)
		d.raise(ana, sec(0))
		d.raise(ben, sec(5))
		again := Action{Kind: Raise, ActorID: ana, At: sec(10), Title: "A second question"}
		d.refused(again, ErrAlreadyOpen)
		d.must(act(Withdraw, ana, 1, sec(20)))
		d.line(2)

		again.At = sec(30)
		var soon *TooSoonError
		if err := d.do(again); !errors.As(err, &soon) || soon.Wait != 30*time.Second {
			t.Fatalf("a hand 30 s after the last one: got %v, want a *TooSoonError with 30 s to wait", err)
		}
		again.At = sec(60)
		d.must(again)
		d.line(2, 3) // a hand raised again starts at the back
	})

	t.Run("FR-QUEUE-8: back two places, then to the end, then skipped (AT-17)", func(t *testing.T) {
		d := newDesk(t)
		for i, who := range []string{ana, ben, cyd, dee, eli} {
			d.raise(who, sec(10*i))
		}
		d.must(pressNext(sec(100))) // Ana's turn, and she does not reply
		d.current(1)
		d.must(act(Away, host, 1, sec(130)))
		d.line(2, 3, 1, 4, 5)

		d.must(pressNext(sec(131)))
		d.must(act(Done, host, 2, sec(190)))
		d.must(pressNext(sec(191)))
		d.must(act(Done, host, 3, sec(250)))
		d.must(pressNext(sec(251))) // Ana again
		d.current(1)
		d.must(act(Away, host, 1, sec(280)))
		d.line(4, 5, 1)

		d.must(pressNext(sec(281)))
		d.must(act(Skip, host, 4, sec(300)))
		d.must(pressNext(sec(301)))
		d.must(act(Done, host, 5, sec(360)))
		d.must(pressNext(sec(361))) // Ana a third time
		d.current(1)
		d.must(act(Away, host, 1, sec(390)))
		d.line()
		if h := d.hand(1); h.Status != Skipped || h.AwayCount != 3 || !h.ClosedAt.Equal(sec(390)) {
			t.Errorf("after the third miss the hand is %+v, want it skipped at 390 s with 3 misses", h)
		}
		d.refused(pressNext(sec(400)), ErrNothingWaiting)
	})

	t.Run("FR-QUEUE-9: I'm here prevents Away, in every turn anew (AT-17)", func(t *testing.T) {
		d := newDesk(t)
		d.raise(ana, sec(0))
		d.raise(ben, sec(10))
		d.must(pressNext(sec(100)))
		d.refused(act(Here, ben, 2, sec(105)), ErrNotAnswering) // it is not Ben's turn
		d.must(act(Away, host, 1, sec(130)))                    // Ana did not reply
		d.line(2, 1)
		d.must(pressNext(sec(131)))
		d.must(act(Here, ben, 2, sec(135)))
		d.refused(act(Away, host, 2, sec(140)), ErrIsHere)
		d.must(act(Done, host, 2, sec(200)))

		d.must(pressNext(sec(201))) // Ana's second turn
		if h := d.hand(1); !h.AckedAt.IsZero() || !h.TakenAt.Equal(sec(201)) || h.AwayCount != 1 {
			t.Errorf("at her second turn the hand is %+v, want it taken at 201 s, with 1 miss and no I'm here yet", h)
		}
		d.must(act(Here, ana, 1, sec(205)))
		d.must(act(Here, ana, 1, sec(209)))
		if h := d.hand(1); !h.AckedAt.Equal(sec(205)) {
			t.Errorf("I'm here twice: the time is %v, want the first press at 205 s", h.AckedAt)
		}
		d.refused(act(Away, host, 1, sec(210)), ErrIsHere)
		d.must(act(Done, host, 1, sec(300)))
	})

	t.Run("FR-SESS-6: last call closes hands, and everyone in line is still answered (AT-18)", func(t *testing.T) {
		d := newDesk(t)
		d.raise(ana, sec(0))
		d.raise(ben, sec(10))
		d.raise(cyd, sec(20))
		lastCall := Action{Kind: LastCall, ActorID: host, At: sec(100), Closed: true}
		d.must(lastCall)
		ask := Action{Kind: Raise, ActorID: dee, At: sec(110), Title: "Is there time for one more?"}
		d.refused(ask, ErrHandsClosed)

		d.must(pressNext(sec(120)))
		d.must(act(Done, host, 1, sec(180)))
		d.must(act(Remove, host, 3, sec(185)))
		d.must(act(Plus, cyd, 2, sec(186)))
		d.must(act(Withdraw, ben, 2, sec(190)))
		d.line()

		lastCall.At, lastCall.Closed = sec(200), false
		d.must(lastCall)
		ask.At = sec(210)
		d.must(ask)
		d.line(4)
	})

	t.Run("FR-HAND-7, FR-HAND-8: +1s and a merge name everyone who asked (AT-19)", func(t *testing.T) {
		d := newDesk(t)
		d.raise(ana, sec(0))
		d.raise(ben, sec(10)) // the same question as Ana's
		d.raise(cyd, sec(20))
		d.must(act(Plus, eli, 1, sec(30)))
		d.must(act(Plus, dee, 1, sec(31)))
		d.must(act(Plus, eli, 1, sec(32))) // a second +1 takes the first back
		d.refused(act(Plus, ana, 1, sec(33)), ErrOwnHand)
		d.must(act(Plus, eli, 2, sec(34)))
		d.must(act(Plus, cyd, 2, sec(35)))
		d.must(act(Plus, ana, 2, sec(36)))
		d.line(1, 2, 3) // the order never follows the votes

		d.must(pressNext(sec(100)))
		d.refused(act(Plus, fay, 1, sec(105)), ErrBadStatus) // no +1 once it is being answered
		d.must(act(Merge, host, 2, sec(110)))
		d.line(3)
		d.must(act(Done, host, 1, sec(200)))

		answered, merged := d.hand(1), d.hand(2)
		if want := []string{ben, cyd, dee, eli}; answered.Status != Answered || !slices.Equal(answered.Plus, want) {
			t.Errorf("the answered hand is %s with the +1s %v, want answered with %v", answered.Status, answered.Plus, want)
		}
		if want := []string{ana, cyd, eli}; merged.Status != Merged || merged.MergedInto != 1 || !slices.Equal(merged.Plus, want) {
			t.Errorf("the merged hand is %s into %d with the +1s %v, want merged into 1 and still %v",
				merged.Status, merged.MergedInto, merged.Plus, want)
		}
		d.raise(ben, sec(300)) // his hand left the queue, so he may ask something else
		d.line(3, 4)
	})

	t.Run("FR-SESS-4, FR-SESS-7: the end waits for the hand being answered, then carries who still waits (AT-11)", func(t *testing.T) {
		d := newDesk(t)
		d.raise(ana, sec(0))
		d.raise(ben, sec(10))
		d.raise(cyd, sec(20))
		d.must(pressNext(sec(100)))
		finish := Action{Kind: End, ActorID: host, At: sec(3000), Carry: true}
		d.refused(finish, ErrAnswering)
		d.must(act(Done, host, 1, sec(3010)))
		finish.At = sec(3020)
		d.must(finish)

		if d.state.Live {
			t.Error("the session is still live after its end")
		}
		for _, id := range []int64{2, 3} {
			if h := d.hand(id); h.Status != Carried || !h.ClosedAt.Equal(sec(3020)) {
				t.Errorf("hand %d is %s, closed at %v; want it carried at 3020 s", id, h.Status, h.ClosedAt)
			}
		}
		d.refused(pressNext(sec(3030)), ErrNotLive)
		d.refused(Action{Kind: Raise, ActorID: dee, At: sec(3040), Title: "Too late?"}, ErrNotLive)
		d.refused(finish, ErrNotLive)
	})
}
