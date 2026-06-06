package evolution

import "fmt"

type StateMachine struct{}

var transitions = map[Status]map[Status]struct{}{
	StatusObserved:      set(StatusWatching, StatusCandidate, StatusRejected, StatusDeferred),
	StatusWatching:      set(StatusCandidate, StatusRejected, StatusDeferred),
	StatusCandidate:     set(StatusProposalDraft, StatusRejected, StatusDeferred),
	StatusProposalDraft: set(StatusReviewReady, StatusRejected, StatusDeferred),
	StatusReviewReady:   set(StatusApproved, StatusRejected, StatusDeferred),
	StatusApproved:      set(StatusTesting, StatusRejected),
	StatusTesting:       set(StatusCanary, StatusRejected, StatusRolledBack),
	StatusCanary:        set(StatusReleased, StatusRolledBack),
	StatusReleased:      set(StatusRolledBack),
	StatusDeferred:      set(StatusWatching, StatusCandidate, StatusRejected),
	StatusRolledBack:    set(StatusCandidate, StatusDeferred),
	StatusRejected:      {},
}

func (StateMachine) CanTransition(from, to Status) bool {
	_, ok := transitions[from][to]
	return ok
}

func (m StateMachine) Transition(from, to Status) error {
	if !m.CanTransition(from, to) {
		return &Error{Code: ErrInvalidTransition, Message: fmt.Sprintf("cannot transition from %s to %s", from, to)}
	}
	return nil
}

func set(values ...Status) map[Status]struct{} {
	out := make(map[Status]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}
