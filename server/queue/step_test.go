package queue

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"
)

// What every test of Step shares: the people, the hands, a Step that also checks it changed
// nothing it was given, and the table runner. The rules themselves are tested kind by kind in
// the other step_*_test.go files, against §4 of notes/phase-2-plan.md.

// The people of these tests. Their ids sort the way their names do, so a +1 list that is in order
// reads in order.
const (
	host = "gh1" // the maintainer who runs the session
	ana  = "gh2"
	ben  = "gh3"
	cyd  = "gh4"
	dee  = "gh5"
	eli  = "gh6"
	fay  = "gh7"
)

// sec is the moment n seconds into the tests' session. Every time in these tests is a whole
// microsecond, as the store keeps them.
func sec(n int) time.Time { return time.Date(2026, 11, 12, 9, 0, n, 0, time.UTC) }

// waits is a hand in the line, raised (and so sorted) at the time given.
func waits(id int64, author string, raised time.Time) Hand {
	return Hand{
		ID: id, AuthorID: author, Title: "A question from " + author, Details: "And what was tried.",
		Status: Waiting, RaisedAt: raised, SortAt: raised,
	}
}

// turn is the hand being answered: the host took it at the time given.
func turn(id int64, author string, raised, taken time.Time) Hand {
	h := waits(id, author, raised)
	h.Status, h.TakenAt, h.TakenBy = Answering, taken, host
	return h
}

// gone is a hand that left the queue with the status given.
func gone(id int64, author string, as Status, at time.Time) Hand {
	h := waits(id, author, at.Add(-time.Minute))
	h.Status, h.ClosedAt = as, at
	return h
}

// liked is the hand with these +1s.
func liked(h Hand, by ...string) Hand {
	h.Plus = by
	return h
}

// live is a live session with these hands.
func live(hands ...Hand) State { return State{Live: true, Hands: hands} }

// act is an action on a hand.
func act(kind Kind, actor string, hand int64, at time.Time) Action {
	return Action{Kind: kind, ActorID: actor, HandID: hand, At: at}
}

// everyKind is every action the contract names.
func everyKind() []Kind {
	return []Kind{Raise, Withdraw, Plus, Here, Next, Skip, Remove, Away, Merge, Done, LastCall, End}
}

// handKinds are the actions that name a hand.
func handKinds() []Kind { return []Kind{Withdraw, Plus, Here, Skip, Remove, Away, Merge, Done} }

// deepCopy is the state with lists of its own.
func deepCopy(s State) State {
	out := s
	out.Hands = slices.Clone(s.Hands)
	for i := range out.Hands {
		out.Hands[i].Plus = slices.Clone(out.Hands[i].Plus)
	}
	return out
}

// with is a copy of the state in which one hand was changed: what Step must return when an
// action touches that hand and nothing else.
func with(t *testing.T, s State, id int64, change func(h *Hand)) State {
	t.Helper()
	out := deepCopy(s)
	for i := range out.Hands {
		if out.Hands[i].ID == id {
			change(&out.Hands[i])
			return out
		}
	}
	t.Fatalf("the test's own state has no hand %d", id)
	return State{}
}

// spare is what roomy writes behind the end of every list.
const spare = "behind the end of the list"

// roomy is a deep copy of the state whose lists have room for one more element, with a mark in
// that room. An append that forgets to copy first lands there, in memory that is the caller's,
// and comparing the state before and after would not show it.
func roomy(s State) State {
	out := s
	if s.Hands != nil {
		out.Hands = make([]Hand, len(s.Hands), len(s.Hands)+1)
		copy(out.Hands, s.Hands)
		out.Hands[:len(out.Hands)+1][len(out.Hands)] = Hand{Title: spare}
	}
	for i, h := range out.Hands {
		if h.Plus == nil {
			continue
		}
		plus := make([]string, len(h.Plus), len(h.Plus)+1)
		copy(plus, h.Plus)
		plus[:len(plus)+1][len(plus)] = spare
		out.Hands[i].Plus = plus
	}
	return out
}

// marked reports whether the marks roomy left are still there.
func marked(s State) bool {
	if s.Hands != nil && s.Hands[:len(s.Hands)+1][len(s.Hands)].Title != spare {
		return false
	}
	for _, h := range s.Hands {
		if h.Plus != nil && h.Plus[:len(h.Plus)+1][len(h.Plus)] != spare {
			return false
		}
	}
	return true
}

// pureStep is Step as every test calls it. It fails the test when Step wrote anywhere in the
// state it was given: the store keeps that state as "before", and a second request may be
// reading it (FR-QUEUE-1).
func pureStep(t *testing.T, s State, a Action) (State, error) {
	t.Helper()
	in := roomy(s)
	before := deepCopy(in)
	got, err := Step(in, a)
	if !reflect.DeepEqual(in, before) {
		t.Fatalf("FR-QUEUE-1: Step(%s) changed the state it was given\n was %+v\n now %+v", a.Kind, before, in)
	}
	if !marked(in) {
		t.Fatalf("FR-QUEUE-1: Step(%s) appended to a list of the state it was given", a.Kind)
	}
	if len(in.Hands) > 0 && len(got.Hands) > 0 && &in.Hands[0] == &got.Hands[0] {
		t.Fatalf("FR-QUEUE-1: the result of Step(%s) shares its list of hands with the state it was given", a.Kind)
	}
	return got, err
}

// sameState fails the test unless the two states are the same session with the same hands. The
// order of the list means nothing (plan §4), so the hands are compared by id.
func sameState(t *testing.T, got, want State) {
	t.Helper()
	if got.Live != want.Live || got.HandsClosed != want.HandsClosed {
		t.Errorf("the session: got live %v, hands closed %v; want live %v, hands closed %v",
			got.Live, got.HandsClosed, want.Live, want.HandsClosed)
	}
	byID := func(hands []Hand) []Hand {
		out := slices.Clone(hands)
		slices.SortStableFunc(out, func(a, b Hand) int { return cmp.Compare(a.ID, b.ID) })
		return out
	}
	g, w := byID(got.Hands), byID(want.Hands)
	if len(g) != len(w) {
		t.Fatalf("got %d hands, want %d\n got %+v\nwant %+v", len(g), len(w), g, w)
	}
	for i := range w {
		if !reflect.DeepEqual(g[i], w[i]) {
			t.Errorf("hand %d\n got %+v\nwant %+v", w[i].ID, g[i], w[i])
		}
	}
}

// lineOf is the queue as the plan defines it: the ids of the hands that wait, by SortAt, then by
// ID, a hand without an id last among equals. It is written out again here so that no test leans
// on the helper Step itself uses.
func lineOf(s State) []int64 {
	var line []Hand
	for _, h := range s.Hands {
		if h.Status == Waiting {
			line = append(line, h)
		}
	}
	sort.SliceStable(line, func(i, j int) bool {
		a, b := line[i], line[j]
		switch {
		case !a.SortAt.Equal(b.SortAt):
			return a.SortAt.Before(b.SortAt)
		case a.ID == 0 || b.ID == 0:
			return a.ID != 0 // the one that has an id stands first
		default:
			return a.ID < b.ID
		}
	})
	ids := make([]int64, len(line))
	for i, h := range line {
		ids[i] = h.ID
	}
	return ids
}

// stepCase is one row of a kind's table: a state, an action, and what Step must answer.
type stepCase struct {
	name   string        // starts with the SRS id the row proves
	in     State         // the state before
	do     Action        // the action
	want   State         // the state after, when the action is allowed
	line   []int64       // and, when not nil, who waits after it, in the queue's order
	refuse error         // or the refusal, compared with errors.Is
	wait   time.Duration // or, for a hand raised too soon, the Wait of the *TooSoonError
}

// runSteps runs a table. Every row also proves that Step left its input alone (pureStep), and
// that a refusal comes without a state.
func runSteps(t *testing.T, cases []stepCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pureStep(t, tt.in, tt.do)
			switch {
			case tt.wait != 0:
				var soon *TooSoonError
				if !errors.As(err, &soon) {
					t.Fatalf("Step = %v, want a *TooSoonError", err)
				}
				if soon.Wait != tt.wait {
					t.Errorf("TooSoonError.Wait = %s, want %s", soon.Wait, tt.wait)
				}
			case tt.refuse != nil:
				if !errors.Is(err, tt.refuse) {
					t.Fatalf("Step = %v, want the refusal %q", err, tt.refuse)
				}
			default:
				if err != nil {
					t.Fatalf("Step refused: %v", err)
				}
				sameState(t, got, tt.want)
				if line := lineOf(got); tt.line != nil && !slices.Equal(line, tt.line) {
					t.Errorf("the line is %v, want %v", line, tt.line)
				}
				return
			}
			if !reflect.DeepEqual(got, State{}) {
				t.Errorf("a refusal came with a state: %+v", got)
			}
		})
	}
}

func TestStepWhenTheSessionIsNotLive(t *testing.T) {
	// A session that is over, with a hand in every open status: no other rule would refuse.
	over := State{Hands: []Hand{waits(1, ana, sec(10)), turn(2, ben, sec(20), sec(60))}}
	var cases []stepCase
	for _, kind := range append(everyKind(), "dance") {
		cases = append(cases,
			stepCase{
				name: fmt.Sprintf("FR-SESS-4: %s on a waiting hand is refused once the session is over", kind),
				in:   over, do: act(kind, ana, 1, sec(600)), refuse: ErrNotLive,
			},
			stepCase{
				name: fmt.Sprintf("FR-SESS-4: %s on the hand being answered is refused once the session is over", kind),
				in:   over, do: act(kind, ben, 2, sec(600)), refuse: ErrNotLive,
			},
			stepCase{
				name: fmt.Sprintf("FR-SESS-4: %s is refused in a session that is over and has no hands", kind),
				in:   State{}, do: Action{Kind: kind, ActorID: host, At: sec(600), Title: "Is anyone here?", Closed: true, Carry: true}, refuse: ErrNotLive,
			},
		)
	}
	runSteps(t, cases)
}

func TestStepWithAnActionItDoesNotKnow(t *testing.T) {
	s := live(waits(1, ana, sec(10)), turn(2, ben, sec(20), sec(60)))
	var cases []stepCase
	for _, kind := range []Kind{"dance", "", "RAISE", "raise "} {
		cases = append(cases, stepCase{
			name: fmt.Sprintf("FR-QUEUE-2: the action %q is not one of the rules", kind),
			in:   s, do: act(kind, ana, 1, sec(600)), refuse: ErrNotImplemented,
		})
	}
	runSteps(t, cases)
}

func TestStepOnAHandThatIsNotInTheQueue(t *testing.T) {
	// One hand that waits, one that is being answered (so that Merge has where to go), and one
	// for every way a hand leaves the queue. The store loads open hands only, but Step's own
	// result still lists what it closed, and stepping that again must answer the same.
	left := []Status{Answered, Skipped, Withdrawn, Removed, Merged, NotReached, Carried}
	s := live(waits(1, ana, sec(10)), turn(2, ben, sec(20), sec(60)))
	for i, status := range left {
		s.Hands = append(s.Hands, gone(int64(10+i), cyd, status, sec(100+i)))
	}
	var cases []stepCase
	for _, kind := range handKinds() {
		cases = append(cases,
			stepCase{
				name: fmt.Sprintf("FR-QUEUE-2: %s on an id no hand has", kind),
				in:   s, do: act(kind, host, 99, sec(600)), refuse: ErrNoSuchHand,
			},
			stepCase{
				name: fmt.Sprintf("FR-QUEUE-2: %s without a hand's id", kind),
				in:   s, do: act(kind, host, 0, sec(600)), refuse: ErrNoSuchHand,
			},
		)
		for i, status := range left {
			cases = append(cases,
				stepCase{
					name: fmt.Sprintf("FR-QUEUE-2: %s by its author on a hand that is %s", kind, status),
					in:   s, do: act(kind, cyd, int64(10+i), sec(600)), refuse: ErrNoSuchHand,
				},
				stepCase{
					name: fmt.Sprintf("FR-QUEUE-2: %s by the maintainer on a hand that is %s", kind, status),
					in:   s, do: act(kind, host, int64(10+i), sec(600)), refuse: ErrNoSuchHand,
				},
			)
		}
	}
	runSteps(t, cases)
}
