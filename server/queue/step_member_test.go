package queue

import (
	"testing"
	"time"
)

// What a member does: raise a hand, withdraw it, +1 someone else's, say "I'm here".

func TestRaise(t *testing.T) {
	now := sec(600)
	ask := Action{
		Kind: Raise, ActorID: ana, At: now,
		Title: "How do I retry only on 5xx?", Details: "The docs show one retry for everything.",
	}
	// The hand that ask adds.
	asked := Hand{AuthorID: ana, Title: ask.Title, Details: ask.Details, Status: Waiting, RaisedAt: now, SortAt: now}
	// The same action by someone whose last hand in this project was raised that long ago.
	last := func(ago time.Duration) Action {
		a := ask
		a.LastRaisedAt = now.Add(-ago)
		return a
	}
	added := func(s State) State {
		out := deepCopy(s)
		out.Hands = append(out.Hands, asked)
		return out
	}
	others := live(turn(1, ben, sec(10), sec(60)), waits(2, cyd, sec(20)))
	anaLeft := live(turn(1, ben, sec(10), sec(60)), gone(2, ana, Withdrawn, sec(300)))
	anaWaits := live(turn(1, ben, sec(10), sec(60)), waits(2, ana, sec(20)))
	anasTurn := live(turn(1, ana, sec(10), sec(60)), waits(2, ben, sec(20)))
	lastCall := State{Live: true, HandsClosed: true}
	lastCallAnaWaits := deepCopy(anaWaits)
	lastCallAnaWaits.HandsClosed = true
	named := ask
	named.HandID = 2

	runSteps(t, []stepCase{
		{
			name: "FR-HAND-1: the first hand of a session waits, without an id, raised and sorted at the action's time",
			in:   live(), do: ask, want: live(asked),
		},
		{
			name: "FR-QUEUE-1: a new hand is added and no other hand changes",
			in:   others, do: ask, want: added(others),
		},
		{
			name: "FR-HAND-1: the new hand has no id, whatever hand the action names",
			in:   others, do: named, want: added(others),
		},
		{
			name: "FR-HAND-2: refused while the author's hand waits",
			in:   anaWaits, do: ask, refuse: ErrAlreadyOpen,
		},
		{
			name: "FR-HAND-2: refused while the author's hand is being answered",
			in:   anasTurn, do: ask, refuse: ErrAlreadyOpen,
		},
		{
			name: "FR-HAND-2: allowed again once the author's hand has left the queue",
			in:   anaLeft, do: ask, want: added(anaLeft),
		},
		{
			name: "FR-SESS-6: refused while hands are closed",
			in:   lastCall, do: ask, refuse: ErrHandsClosed,
		},
		{
			name: "FR-SESS-6: closed hands are said before an open hand and before the 60 seconds",
			in:   lastCallAnaWaits, do: last(time.Second), refuse: ErrHandsClosed,
		},
		{
			name: "FR-HAND-2: an open hand is said before the 60 seconds",
			in:   anaWaits, do: last(time.Second), refuse: ErrAlreadyOpen,
		},
		{
			name: "FR-HAND-6: refused 20 s after the author's last hand, with 40 s left to wait",
			in:   others, do: last(20 * time.Second), wait: 40 * time.Second,
		},
		{
			name: "FR-HAND-6: refused one microsecond early",
			in:   others, do: last(RaiseEvery - time.Microsecond), wait: time.Microsecond,
		},
		{
			name: "FR-HAND-6: a last hand of this very moment waits the full 60 s",
			in:   others, do: last(0), wait: RaiseEvery,
		},
		{
			name: "FR-HAND-6: a last hand later than now, from a server whose clock runs ahead, is still too soon",
			in:   others, do: last(-2 * time.Second), wait: RaiseEvery + 2*time.Second,
		},
		{
			name: "FR-HAND-6: allowed exactly 60 s after the last hand",
			in:   others, do: last(RaiseEvery), want: added(others),
		},
		{
			name: "FR-HAND-6: allowed an hour after the last hand, in a session where the author's old hand is closed",
			in:   anaLeft, do: last(time.Hour), want: added(anaLeft),
		},
	})
}

func TestWithdraw(t *testing.T) {
	now := sec(600)
	s := live(waits(1, ana, sec(10)), turn(2, ben, sec(20), sec(60)), liked(waits(3, cyd, sec(30)), ana, dee))
	withdrawn := func(h *Hand) { h.Status, h.ClosedAt = Withdrawn, now }

	runSteps(t, []stepCase{
		{
			name: "FR-HAND-4: the author takes back a hand that waits",
			in:   s, do: act(Withdraw, ana, 1, now), want: with(t, s, 1, withdrawn),
		},
		{
			name: "FR-HAND-4: a hand with +1s is withdrawn like any other and keeps them",
			in:   s, do: act(Withdraw, cyd, 3, now), want: with(t, s, 3, withdrawn),
		},
		{
			name: "FR-HAND-4: another member can't withdraw it",
			in:   s, do: act(Withdraw, ben, 1, now), refuse: ErrNotYours,
		},
		{
			name: "FR-HAND-4: the maintainer can't either (theirs is Remove)",
			in:   s, do: act(Withdraw, host, 1, now), refuse: ErrNotYours,
		},
		{
			name: "FR-HAND-4: not once it is being answered",
			in:   s, do: act(Withdraw, ben, 2, now), refuse: ErrBadStatus,
		},
		{
			name: "FR-HAND-4: on someone else's hand that is being answered, whose it is comes first",
			in:   s, do: act(Withdraw, ana, 2, now), refuse: ErrNotYours,
		},
	})
}

func TestPlus(t *testing.T) {
	now := sec(600)
	s := live(liked(waits(1, ana, sec(10)), ben, fay), turn(2, cyd, sec(20), sec(60)), waits(3, dee, sec(30)), liked(waits(4, eli, sec(40)), fay))
	// likedBy sets a hand's +1 list. Without ids it is nil: a hand nobody +1'd has no list.
	likedBy := func(ids ...string) func(h *Hand) { return func(h *Hand) { h.Plus = ids } }

	runSteps(t, []stepCase{
		{
			name: "FR-HAND-7: the first +1 on a hand",
			in:   s, do: act(Plus, eli, 3, now), want: with(t, s, 3, likedBy(eli)),
		},
		{
			name: "FR-HAND-7: a +1 goes where its user id sorts, here between two others",
			in:   s, do: act(Plus, dee, 1, now), want: with(t, s, 1, likedBy(ben, dee, fay)),
		},
		{
			name: "FR-HAND-7: a +1 whose user id sorts first",
			in:   s, do: act(Plus, host, 1, now), want: with(t, s, 1, likedBy(host, ben, fay)),
		},
		{
			name: "FR-HAND-7: a +1 whose user id sorts last",
			in:   s, do: act(Plus, "gh8", 1, now), want: with(t, s, 1, likedBy(ben, fay, "gh8")),
		},
		{
			name: "FR-HAND-7: user ids sort as text, the way the database sorts them: gh10 before gh3",
			in:   s, do: act(Plus, "gh10", 1, now), want: with(t, s, 1, likedBy("gh10", ben, fay)),
		},
		{
			name: "FR-HAND-7: a second +1 takes the first back",
			in:   s, do: act(Plus, ben, 1, now), want: with(t, s, 1, likedBy(fay)),
		},
		{
			name: "FR-HAND-7: taking back the last of the list",
			in:   s, do: act(Plus, fay, 1, now), want: with(t, s, 1, likedBy(ben)),
		},
		{
			name: "FR-HAND-7: taking back the only +1 leaves no list",
			in:   s, do: act(Plus, fay, 4, now), want: with(t, s, 4, likedBy()),
		},
		{
			name: "FR-HAND-7: no +1 on one's own hand",
			in:   s, do: act(Plus, ana, 1, now), refuse: ErrOwnHand,
		},
		{
			name: "FR-HAND-7: no +1 on the hand being answered",
			in:   s, do: act(Plus, ana, 2, now), refuse: ErrBadStatus,
		},
		{
			name: "FR-HAND-7: on one's own hand that is being answered, own hand comes first",
			in:   s, do: act(Plus, cyd, 2, now), refuse: ErrOwnHand,
		},
	})
}

func TestHere(t *testing.T) {
	now := sec(600)
	s := live(turn(1, ana, sec(10), sec(500)), waits(2, ben, sec(20)))
	said := with(t, s, 1, func(h *Hand) { h.AckedAt = sec(520) })

	runSteps(t, []stepCase{
		{
			name: "FR-QUEUE-9: the author of the hand being answered says I'm here",
			in:   s, do: act(Here, ana, 1, now), want: with(t, s, 1, func(h *Hand) { h.AckedAt = now }),
		},
		{
			name: "FR-QUEUE-9: a second press keeps the first time",
			in:   said, do: act(Here, ana, 1, now), want: said,
		},
		{
			name: "FR-QUEUE-9: another member can't say it for them",
			in:   s, do: act(Here, ben, 1, now), refuse: ErrNotYours,
		},
		{
			name: "FR-QUEUE-9: the maintainer can't say it for them",
			in:   s, do: act(Here, host, 1, now), refuse: ErrNotYours,
		},
		{
			name: "FR-QUEUE-9: not before one's turn",
			in:   s, do: act(Here, ben, 2, now), refuse: ErrNotAnswering,
		},
		{
			name: "FR-QUEUE-9: on someone else's hand that waits, whose it is comes first",
			in:   s, do: act(Here, ana, 2, now), refuse: ErrNotYours,
		},
	})
}
