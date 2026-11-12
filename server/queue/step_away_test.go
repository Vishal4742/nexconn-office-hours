package queue

import (
	"fmt"
	"testing"
	"time"
)

// Away (FR-QUEUE-8): the author of the hand being answered is not there. The first miss sends
// the hand two places behind, the second to the end, and the third skips it. In these tests the
// hand is always Fay's, with id 9, and it is marked away at 160 s.

// awayLine is a session in which Fay's hand is being answered after some misses, while others
// wait: hands 1, 2, 3… sorted at 10 s, 20 s, 30 s…
func awayLine(others, missesSoFar int) State {
	people := []string{ana, ben, cyd, dee, eli}
	s := live()
	for i := range others {
		s.Hands = append(s.Hands, waits(int64(i+1), people[i], sec(10*(i+1))))
	}
	fays := turn(9, fay, sec(1), sec(100))
	fays.AwayCount = missesSoFar
	s.Hands = append(s.Hands, fays)
	return s
}

// sentBack is the state after Away put hand 9 back into the line at sortAt, on that miss.
func sentBack(t *testing.T, s State, miss int, sortAt time.Time) State {
	t.Helper()
	return with(t, s, 9, func(h *Hand) {
		h.AwayCount, h.Status, h.SortAt = miss, Waiting, sortAt
		h.TakenAt, h.TakenBy, h.AckedAt = time.Time{}, "", time.Time{}
	})
}

func TestAwayByMissAndLine(t *testing.T) {
	now := sec(160)
	var cases []stepCase
	for _, tt := range []struct {
		others, miss int
		sortAt       time.Time // where hand 9 is sorted afterwards; zero when it is skipped
		line         []int64   // who waits afterwards, in order
		what         string
	}{
		{0, 1, now, []int64{9}, "alone in the line"},
		{1, 1, now, []int64{1, 9}, "the end, the one place there is"},
		{2, 1, now, []int64{1, 2, 9}, "the end, which is two places behind"},
		{3, 1, sec(25), []int64{1, 2, 9, 3}, "two places behind, halfway between the second and the third"},
		{5, 1, sec(25), []int64{1, 2, 9, 3, 4, 5}, "two places behind, halfway between the second and the third"},
		{0, 2, now, []int64{9}, "alone in the line"},
		{1, 2, now, []int64{1, 9}, "the end"},
		{2, 2, now, []int64{1, 2, 9}, "the end"},
		{3, 2, now, []int64{1, 2, 3, 9}, "the end"},
		{5, 2, now, []int64{1, 2, 3, 4, 5, 9}, "the end"},
		{0, 3, time.Time{}, []int64{}, "skipped"},
		{1, 3, time.Time{}, []int64{1}, "skipped"},
		{2, 3, time.Time{}, []int64{1, 2}, "skipped"},
		{3, 3, time.Time{}, []int64{1, 2, 3}, "skipped"},
		{5, 3, time.Time{}, []int64{1, 2, 3, 4, 5}, "skipped"},
	} {
		s := awayLine(tt.others, tt.miss-1)
		want := sentBack(t, s, tt.miss, tt.sortAt)
		if tt.sortAt.IsZero() { // the hand keeps who took it and when, like any hand skipped during its turn
			want = with(t, s, 9, func(h *Hand) { h.AwayCount, h.Status, h.ClosedAt = tt.miss, Skipped, now })
		}
		cases = append(cases, stepCase{
			name: fmt.Sprintf("FR-QUEUE-8: miss %d with %d waiting: %s", tt.miss, tt.others, tt.what),
			in:   s, do: act(Away, host, 9, now), want: want, line: tt.line,
		})
	}
	runSteps(t, cases)
}

func TestAwayHalfwayToTheMicrosecond(t *testing.T) {
	now := sec(160)
	var cases []stepCase
	for _, tt := range []struct {
		gap  time.Duration // from the second waiting hand to the third
		move time.Duration // from the second waiting hand to hand 9 afterwards; 0 when it goes to the end
		what string
	}{
		{10 * time.Second, 5 * time.Second, "10 s apart: halfway"},
		{7 * time.Microsecond, 3 * time.Microsecond, "7 µs apart: halfway, cut to the whole microsecond"},
		{3 * time.Microsecond, time.Microsecond, "3 µs apart: halfway, cut to the whole microsecond"},
		{2 * time.Microsecond, time.Microsecond, "2 µs apart: the one microsecond between them"},
		{time.Microsecond, 0, "1 µs apart: no microsecond lies between them, so the end"},
		{0, 0, "the same moment: no microsecond lies between them, so the end"},
	} {
		s := live(waits(1, ana, sec(10)), waits(2, ben, sec(20)), waits(3, cyd, sec(20).Add(tt.gap)), waits(4, dee, sec(40)), turn(9, fay, sec(1), sec(100)))
		want, line := sentBack(t, s, 1, sec(20).Add(tt.move)), []int64{1, 2, 9, 3, 4}
		if tt.move == 0 {
			want, line = sentBack(t, s, 1, now), []int64{1, 2, 3, 4, 9}
		}
		cases = append(cases, stepCase{
			name: "FR-QUEUE-8: the second and the third are " + tt.what,
			in:   s, do: act(Away, host, 9, now), want: want, line: line,
		})
	}
	runSteps(t, cases)
}

func TestAwayToTheEnd(t *testing.T) {
	now := sec(160)
	fays := turn(9, fay, sec(1), sec(100))
	second := fays
	second.AwayCount = 1
	later := now.Add(10 * time.Second)

	earlier := live(waits(1, ana, sec(10)), waits(2, ben, sec(20)), second)
	sameMoment := live(waits(1, ana, sec(10)), waits(2, ben, now), second)
	clockAhead := live(waits(1, ana, sec(10)), waits(2, ben, later), second)
	clockAheadNoRoom := live(waits(1, ana, sec(10)), waits(2, ben, sec(20)), waits(3, cyd, sec(20).Add(time.Microsecond)), waits(4, dee, later), fays)
	// Dee's hand left the queue; it is sorted between the others and later than now, and counts
	// neither as a place in the line nor as its end.
	left := gone(4, dee, Skipped, later)
	left.SortAt = sec(15)
	leftLast := gone(5, eli, Withdrawn, later)
	leftLast.SortAt = later
	someLeft := live(waits(1, ana, sec(10)), left, waits(2, ben, sec(20)), waits(3, cyd, sec(30)), leftLast, fays)
	someLeftSecond := live(waits(1, ana, sec(10)), left, leftLast, second)

	runSteps(t, []stepCase{
		{
			name: "FR-QUEUE-8: the end is now when everyone who waits is sorted earlier",
			in:   earlier, do: act(Away, host, 9, now), want: sentBack(t, earlier, 2, now), line: []int64{1, 2, 9},
		},
		{
			name: "FR-QUEUE-8: the end is one microsecond after a hand that is sorted at this very moment",
			in:   sameMoment, do: act(Away, host, 9, now), want: sentBack(t, sameMoment, 2, now.Add(time.Microsecond)), line: []int64{1, 2, 9},
		},
		{
			name: "FR-QUEUE-8: the end is one microsecond after a hand that is sorted later than now (another server's clock)",
			in:   clockAhead, do: act(Away, host, 9, now), want: sentBack(t, clockAhead, 2, later.Add(time.Microsecond)), line: []int64{1, 2, 9},
		},
		{
			name: "FR-QUEUE-8: a first miss with no room two places behind goes to that same end",
			in:   clockAheadNoRoom, do: act(Away, host, 9, now), want: sentBack(t, clockAheadNoRoom, 1, later.Add(time.Microsecond)), line: []int64{1, 2, 3, 4, 9},
		},
		{
			name: "FR-QUEUE-8: hands that left the queue are not places in the line",
			in:   someLeft, do: act(Away, host, 9, now), want: sentBack(t, someLeft, 1, sec(25)), line: []int64{1, 2, 9, 3},
		},
		{
			name: "FR-QUEUE-8: hands that left the queue are not its end",
			in:   someLeftSecond, do: act(Away, host, 9, now), want: sentBack(t, someLeftSecond, 2, now), line: []int64{1, 9},
		},
	})
}

func TestAwayRefused(t *testing.T) {
	now := sec(160)
	s := awayLine(3, 0)
	saidHere := with(t, s, 9, func(h *Hand) { h.AckedAt = sec(120) })
	saidHereAfterTwoMisses := with(t, saidHere, 9, func(h *Hand) { h.AwayCount = 2 })
	// A waiting hand never carries an "I'm here" (Away and Next clear it). This one does, to show
	// which refusal comes first.
	waitingButHere := with(t, s, 1, func(h *Hand) { h.AckedAt = sec(120) })

	runSteps(t, []stepCase{
		{
			name: "FR-QUEUE-8: a hand that waits can't be marked away",
			in:   s, do: act(Away, host, 1, now), refuse: ErrNotAnswering,
		},
		{
			name: "FR-QUEUE-9: refused once the author said I'm here",
			in:   saidHere, do: act(Away, host, 9, now), refuse: ErrIsHere,
		},
		{
			name: "FR-QUEUE-9: I'm here also keeps a hand from being skipped on a third miss",
			in:   saidHereAfterTwoMisses, do: act(Away, host, 9, now), refuse: ErrIsHere,
		},
		{
			name: "FR-QUEUE-8: on a hand that waits, not being answered is said before I'm here",
			in:   waitingButHere, do: act(Away, host, 1, now), refuse: ErrNotAnswering,
		},
	})
}
