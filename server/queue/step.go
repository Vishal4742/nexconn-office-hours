package queue

// Step is the one rule function: the state after the action, or why the action is refused. It
// never changes its argument (the Hands slice of the result is a new one).
//
// Not built yet: the queue-rules worker of Phase 2 owns this file and its tests
// (notes/phase-2-plan.md, worker 1).
func Step(s State, a Action) (State, error) {
	return State{}, ErrNotImplemented
}
