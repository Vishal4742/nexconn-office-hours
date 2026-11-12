package queue

import (
	"fmt"
	"testing"
	"time"
)

// What a maintainer does: Next, Skip, Remove, Done, Merge, Last call, End. Away has its own file.

func TestNext(t *testing.T) {
	now := sec(600)
	press := Action{Kind: Next, ActorID: host, At: now}
	taken := func(h *Hand) { h.Status, h.TakenAt, h.TakenBy = Answering, now, host }
	// sortedAt is a hand whose place in the line is no longer its raise time.
	sortedAt := func(h Hand, at time.Time) Hand {
		h.SortAt = at
		return h
	}

	listedLast := live(waits(3, cyd, sec(30)), waits(2, ben, sec(20)), waits(1, ana, sec(10)))
	youngerID := live(waits(1, ana, sec(30)), waits(2, ben, sec(10)))
	movedBack := live(sortedAt(waits(1, ana, sec(1)), sec(100)), waits(2, ben, sec(50)))
	sameMoment := live(waits(7, ana, sec(10)), waits(4, ben, sec(10)))
	noIDYet := live(waits(0, ana, sec(10)), waits(9, ben, sec(10)))
	manyPlus := live(waits(1, ana, sec(10)), liked(waits(2, ben, sec(20)), cyd, dee, eli))
	saidHere := live(waits(1, ana, sec(10)))
	saidHere.Hands[0].AckedAt = sec(100)
	someLeft := live(gone(1, ana, Withdrawn, sec(5)), gone(2, ben, Skipped, sec(6)), waits(3, cyd, sec(20)))
	named := press
	named.HandID = 3

	runSteps(t, []stepCase{
		{
			name: "FR-QUEUE-3: the oldest waiting hand is taken, at the action's time, by whoever pressed Next",
			in:   listedLast, do: press, want: with(t, listedLast, 1, taken),
		},
		{
			name: "FR-QUEUE-1: the order is sort_at, not the id",
			in:   youngerID, do: press, want: with(t, youngerID, 2, taken),
		},
		{
			name: "FR-QUEUE-1: the order is sort_at, not the time the hand was raised",
			in:   movedBack, do: press, want: with(t, movedBack, 2, taken),
		},
		{
			name: "FR-QUEUE-1: hands of the same moment go by id",
			in:   sameMoment, do: press, want: with(t, sameMoment, 4, taken),
		},
		{
			name: "FR-QUEUE-1: a hand that has no id yet goes last among hands of the same moment",
			in:   noIDYet, do: press, want: with(t, noIDYet, 9, taken),
		},
		{
			name: "FR-HAND-7: +1s never move a hand forward",
			in:   manyPlus, do: press, want: with(t, manyPlus, 1, taken),
		},
		{
			name: "FR-QUEUE-9: an old I'm here does not count for the new turn",
			in:   saidHere, do: press, want: with(t, saidHere, 1, func(h *Hand) { taken(h); h.AckedAt = time.Time{} }),
		},
		{
			name: "FR-QUEUE-3: Next takes the first in line, whatever hand the action names",
			in:   listedLast, do: named, want: with(t, listedLast, 1, taken),
		},
		{
			name: "FR-QUEUE-3: hands that left the queue are passed over",
			in:   someLeft, do: press, want: with(t, someLeft, 3, taken),
		},
		{
			name: "FR-QUEUE-3: refused while a hand is being answered",
			in:   live(turn(1, ana, sec(10), sec(60)), waits(2, ben, sec(20))), do: press, refuse: ErrAnswering,
		},
		{
			name: "FR-QUEUE-3: a hand being answered is said before an empty line",
			in:   live(turn(1, ana, sec(10), sec(60))), do: press, refuse: ErrAnswering,
		},
		{
			name: "FR-QUEUE-3: refused when the session has no hands",
			in:   live(), do: press, refuse: ErrNothingWaiting,
		},
		{
			name: "FR-QUEUE-3: refused when every hand has left the queue",
			in:   live(gone(1, ana, Answered, sec(5)), gone(2, ben, Removed, sec(6))), do: press, refuse: ErrNothingWaiting,
		},
	})
}

// closingRows are the rows Skip and Remove share: a maintainer takes a hand out of the queue,
// whether it waits or is being answered.
func closingRows(t *testing.T, kind Kind, to Status, id string) []stepCase {
	t.Helper()
	now := sec(600)
	s := live(turn(1, ana, sec(10), sec(500)), liked(waits(2, ben, sec(20)), cyd), waits(3, dee, sec(30)))
	s.Hands[0].AckedAt = sec(510)
	closed := func(h *Hand) { h.Status, h.ClosedAt = to, now }
	return []stepCase{
		{
			name: fmt.Sprintf("%s: %s takes a waiting hand out of the queue", id, kind),
			in:   s, do: act(kind, host, 2, now), want: with(t, s, 2, closed),
		},
		{
			name: fmt.Sprintf("%s: %s closes the hand being answered, which keeps who took it and when", id, kind),
			in:   s, do: act(kind, host, 1, now), want: with(t, s, 1, closed),
		},
		{
			name: fmt.Sprintf("%s: %s does not ask who presses, because the guard did", id, kind),
			in:   s, do: act(kind, fay, 3, now), want: with(t, s, 3, closed),
		},
	}
}

func TestSkip(t *testing.T) { runSteps(t, closingRows(t, Skip, Skipped, "FR-QUEUE-3")) }

func TestRemove(t *testing.T) { runSteps(t, closingRows(t, Remove, Removed, "FR-MOD-1")) }

func TestDone(t *testing.T) {
	now := sec(600)
	s := live(liked(turn(1, ana, sec(10), sec(500)), ben, cyd), waits(2, ben, sec(20)))
	s.Hands[0].AckedAt = sec(510)

	runSteps(t, []stepCase{
		{
			name: "FR-QUEUE-3: the hand being answered is answered, and keeps its turn, its I'm here and its +1s",
			in:   s, do: act(Done, host, 1, now), want: with(t, s, 1, func(h *Hand) { h.Status, h.ClosedAt = Answered, now }),
		},
		{
			name: "FR-QUEUE-2: a hand that waits can't be answered before its turn",
			in:   s, do: act(Done, host, 2, now), refuse: ErrNotAnswering,
		},
	})
}

func TestMerge(t *testing.T) {
	now := sec(600)
	// merged is the state after hand id joined hand 1, whose +1 list is then askers.
	merged := func(s State, id int64, askers ...string) State {
		out := with(t, s, id, func(h *Hand) { h.Status, h.MergedInto, h.ClosedAt = Merged, 1, now })
		return with(t, out, 1, func(h *Hand) { h.Plus = askers })
	}
	// Ana's hand is being answered. Ben's is the same question, and Ana herself had +1'd it.
	s := live(
		liked(turn(1, ana, sec(10), sec(500)), eli),
		liked(waits(2, ben, sec(20)), ana, dee, eli),
		waits(3, cyd, sec(30)),
	)
	benLikedIt := with(t, s, 1, func(h *Hand) { h.Plus = []string{ben, eli} })
	nobodyLikedIt := with(t, s, 1, func(h *Hand) { h.Plus = nil })
	outOfOrder := live(
		liked(turn(1, ana, sec(10), sec(500)), dee, fay),
		liked(waits(2, cyd, sec(20)), ben, eli, "gh8"),
	)
	nobodysTurn := live(waits(2, ben, sec(20)), waits(3, cyd, sec(30)))

	runSteps(t, []stepCase{
		{
			name: "FR-HAND-8: a waiting hand joins the one being answered, and its author asks along",
			in:   s, do: act(Merge, host, 3, now), want: merged(s, 3, cyd, eli),
		},
		{
			name: "FR-HAND-8: its +1s are copied over, each person once and never the author of the hand being answered",
			in:   s, do: act(Merge, host, 2, now), want: merged(s, 2, ben, dee, eli),
		},
		{
			name: "FR-HAND-8: an author who had +1'd the hand being answered is counted once",
			in:   benLikedIt, do: act(Merge, host, 2, now), want: merged(benLikedIt, 2, ben, dee, eli),
		},
		{
			name: "FR-HAND-8: the hand being answered had no +1 yet",
			in:   nobodyLikedIt, do: act(Merge, host, 3, now), want: merged(nobodyLikedIt, 3, cyd),
		},
		{
			name: "FR-HAND-8: the +1s stay sorted by user id",
			in:   outOfOrder, do: act(Merge, host, 2, now), want: merged(outOfOrder, 2, ben, cyd, dee, eli, fay, "gh8"),
		},
		{
			name: "FR-HAND-8: the hand being answered can't be merged, not even into itself",
			in:   s, do: act(Merge, host, 1, now), refuse: ErrBadStatus,
		},
		{
			name: "FR-HAND-8: refused when no hand is being answered",
			in:   nobodysTurn, do: act(Merge, host, 3, now), refuse: ErrNotAnswering,
		},
	})
}

func TestLastCall(t *testing.T) {
	now := sec(600)
	open := live(turn(1, ana, sec(10), sec(500)), liked(waits(2, ben, sec(20)), cyd))
	closed := deepCopy(open)
	closed.HandsClosed = true
	call := func(on bool) Action { return Action{Kind: LastCall, ActorID: host, At: now, Closed: on} }

	runSteps(t, []stepCase{
		{name: "FR-SESS-6: last call closes hands and changes no hand", in: open, do: call(true), want: closed},
		{name: "FR-SESS-6: hands are opened again", in: closed, do: call(false), want: open},
		{name: "FR-SESS-6: last call twice leaves hands closed", in: closed, do: call(true), want: closed},
		{name: "FR-SESS-6: opening hands that are open leaves them open", in: open, do: call(false), want: open},
		{name: "FR-SESS-6: last call in a session without hands", in: live(), do: call(true), want: State{Live: true, HandsClosed: true}},
	})
}

func TestEnd(t *testing.T) {
	now := sec(3600)
	finish := func(carry bool) Action { return Action{Kind: End, ActorID: host, At: now, Carry: carry} }
	// ended is the state after the end: every hand that waited (1 and 2) left with the status given.
	ended := func(s State, as Status) State {
		out := deepCopy(s)
		out.Live = false
		for _, id := range []int64{1, 2} {
			out = with(t, out, id, func(h *Hand) { h.Status, h.ClosedAt = as, now })
		}
		return out
	}
	s := live(waits(1, ana, sec(10)), liked(waits(2, ben, sec(20)), cyd), gone(3, dee, Answered, sec(400)), gone(4, eli, Skipped, sec(500)))
	s.Hands[0].AwayCount = 1
	lastCall := deepCopy(s)
	lastCall.HandsClosed = true
	midAnswer := live(turn(1, ana, sec(10), sec(3000)), waits(2, ben, sec(20)))

	runSteps(t, []stepCase{
		{
			name: "FR-SESS-4: with carry-over, every waiting hand is carried to the next session (FR-SESS-7), and closed hands stay as they are",
			in:   s, do: finish(true), want: ended(s, Carried),
		},
		{
			name: "FR-SESS-4: without carry-over, every waiting hand was not reached",
			in:   s, do: finish(false), want: ended(s, NotReached),
		},
		{
			name: "FR-SESS-4: ending during last call leaves hands closed",
			in:   lastCall, do: finish(true), want: ended(lastCall, Carried),
		},
		{
			name: "FR-SESS-4: a session without hands ends",
			in:   live(), do: finish(true), want: State{},
		},
		{
			name: "FR-SESS-4: refused while a hand is being answered, with carry-over",
			in:   midAnswer, do: finish(true), refuse: ErrAnswering,
		},
		{
			name: "FR-SESS-4: refused while a hand is being answered, without carry-over",
			in:   midAnswer, do: finish(false), refuse: ErrAnswering,
		},
	})
}

func TestEndIsFinal(t *testing.T) {
	// The state is the one End itself returned, with the carried hands still in its list.
	s := live(waits(1, ana, sec(10)), waits(2, ben, sec(20)))
	over, err := pureStep(t, s, Action{Kind: End, ActorID: host, At: sec(3600), Carry: true})
	if err != nil {
		t.Fatalf("End refused: %v", err)
	}
	var cases []stepCase
	for _, kind := range everyKind() {
		cases = append(cases, stepCase{
			name: fmt.Sprintf("FR-SESS-4: after the end, %s is refused", kind),
			in:   over, do: Action{Kind: kind, ActorID: ana, HandID: 1, At: sec(3700), Title: "One more question"}, refuse: ErrNotLive,
		})
	}
	runSteps(t, cases)
}
