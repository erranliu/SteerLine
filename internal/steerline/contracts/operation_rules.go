package contracts

var allowedOperationTransitions = map[string]map[string]bool{
	"pending":         {"in_flight": true, "rejected": true},
	"in_flight":       {"accepted": true, "confirmed": true, "rejected": true, "outcome_unknown": true},
	"accepted":        {"confirmed": true, "rejected": true, "outcome_unknown": true},
	"outcome_unknown": {"accepted": true, "confirmed": true, "rejected": true},
}

func OperationTransitionAllowed(before, after string) bool {
	return allowedOperationTransitions[before][after]
}
