package queue

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// noHand is an id no hand of the random sessions ever gets.
const noHand = 1_000_000

// randomPause is how long the room waits before its next action. Often that is nothing at all or
// one microsecond, so that hands share a moment or leave no room between them.
func randomPause(rng *rand.Rand) time.Duration {
	switch rng.IntN(5) {
	case 0:
		return 0
	case 1:
		return time.Microsecond
	}
	return time.Duration(rng.IntN(20_000_000)) * time.Microsecond
}

// randomHand picks the hand an action names: mostly an open one, and for what is done to the hand
// being answered mostly that one. Now and then it is a hand that left the queue, or nobody's.
func randomHand(rng *rand.Rand, s State, kind Kind) Hand {
	var open, current []Hand
	for _, h := range s.Hands {
		if h.Status.IsOpen() {
			open = append(open, h)
		}
		if h.Status == Answering {
			current = append(current, h)
		}
	}
	forCurrent := kind == Here || kind == Away || kind == Done
	switch {
	case len(s.Hands) == 0 || rng.IntN(25) == 0:
		return Hand{ID: noHand}
	case forCurrent && len(current) > 0 && rng.IntN(5) > 0:
		return current[0]
	case len(open) > 0 && rng.IntN(12) > 0:
		return open[rng.IntN(len(open))]
	}
	return s.Hands[rng.IntN(len(s.Hands))]
}

// randomAction is something someone in the room might do next, sensible or not.
func randomAction(rng *rand.Rand, s State, now time.Time) Action {
	people := []string{ana, ben, cyd, dee, eli, fay, "gh8", "gh9", "gh10", "gh11"}
	common := []Kind{
		Raise, Raise, Raise, Raise, Raise, Raise, Plus, Plus, Plus, Withdraw, Here, Here,
		Next, Next, Next, Done, Done, Away, Away, Skip, Remove, Merge,
	}
	a := Action{
		At: now, ActorID: people[rng.IntN(len(people))], Title: "A question",
		Closed: rng.IntN(4) == 0, Carry: rng.IntN(2) == 0,
	}
	switch n := rng.IntN(300); {
	case n == 0:
		a.Kind = End
	case n == 1:
		a.Kind = "dance" // no such action
	case n < 6:
		a.Kind = LastCall
	default:
		a.Kind = common[rng.IntN(len(common))]
	}
	h := randomHand(rng, s, a.Kind)
	a.HandID = h.ID
	if (a.Kind == Withdraw || a.Kind == Here) && h.AuthorID != "" && rng.IntN(4) > 0 {
		a.ActorID = h.AuthorID // mostly the hand's own author, who alone may
	}
	return a
}

// TestRandomSessions sends a few thousand random actions through Step, each on the state the one
// before it left, and checks after every one that Step allowed what must hold in any session
// (checkStep): one hand at most is being answered, nobody has two open hands, no hand is lost or
// made up, ids and questions stay what they were. The seed is fixed, so a failure runs again the
// same way.
func TestRandomSessions(t *testing.T) {
	rng := rand.New(rand.NewPCG(2026, 1112))
	contract := []error{
		ErrNotImplemented, ErrNotLive, ErrHandsClosed, ErrAlreadyOpen, ErrNoSuchHand, ErrNotYours,
		ErrOwnHand, ErrNothingWaiting, ErrAnswering, ErrNotAnswering, ErrIsHere, ErrBadStatus,
	}
	d := newDesk(t)
	now := sec(0)
	allowed, refused := map[Kind]int{}, map[error]int{}
	tooSoon, longest := 0, 0

	for range 6000 {
		now = now.Add(randomPause(rng))
		if !d.state.Live && rng.IntN(4) == 0 {
			d.state = State{Live: true} // the next session starts; ids and last hands go on
		}
		wasLive := d.state.Live
		a := randomAction(rng, d.state, now)
		err := d.do(a)

		var soon *TooSoonError
		switch {
		case err == nil:
			allowed[a.Kind]++
		case errors.As(err, &soon):
			tooSoon++
			if soon.Wait <= 0 || soon.Wait > RaiseEvery {
				t.Fatalf("FR-HAND-6: a wait of %s, with a clock that only moves forward", soon.Wait)
			}
		default:
			i := slices.IndexFunc(contract, func(e error) bool { return errors.Is(err, e) })
			if i < 0 {
				t.Fatalf("%s was refused with an error the contract does not name: %v", a.Kind, err)
			}
			refused[contract[i]]++
		}
		if !wasLive && !errors.Is(err, ErrNotLive) {
			t.Fatalf("FR-SESS-4: %s in a session that is over: got %v, want %v", a.Kind, err, ErrNotLive)
		}

		longest = max(longest, len(lineOf(d.state)))
		// The store loads open hands only; Step's own result still lists what it closed. Half
		// the time the next action steps the one kind of state, half the time the other.
		if rng.IntN(2) == 0 {
			d.state.Hands = slices.DeleteFunc(d.state.Hands, func(h Hand) bool { return !h.Status.IsOpen() })
		}
	}

	// A loop that never got an action through, or never met a refusal, proves nothing about it.
	for _, kind := range everyKind() {
		if allowed[kind] == 0 {
			t.Errorf("no %s was allowed in the whole run", kind)
		}
	}
	for _, e := range contract {
		if refused[e] == 0 {
			t.Errorf("nothing was refused with %q in the whole run", e)
		}
	}
	if tooSoon == 0 {
		t.Error("no hand was raised too soon in the whole run")
	}
	if longest < 5 {
		t.Errorf("the line never grew past %d hands", longest)
	}
	t.Logf("allowed %v, refused %v, too soon %d, longest line %d", allowed, refused, tooSoon, longest)
}
