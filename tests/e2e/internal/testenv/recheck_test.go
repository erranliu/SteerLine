package testenv

import "testing"

func TestEnvironmentRecheckWaitsForOriginalAsyncOperationWithoutReplay(t *testing.T) {
	rechecked, calls := false, 0
	recheck := func() error { calls++; return nil }
	for _, reason := range []string{"resource_preparation_unverified", "resource_preparation_unverified", "agent_input_unverified"} {
		handled, err := recheckNativeEnvironmentOnce(reason, &rechecked, recheck)
		if !handled || err != nil {
			t.Fatal("original asynchronous environment recheck stopped waiting")
		}
	}
	if calls != 1 {
		t.Fatalf("retained blocker replayed the original input: rechecks=%d", calls)
	}
	if handled, err := recheckNativeEnvironmentOnce("unsupported_agent", &rechecked, recheck); handled || err != nil || calls != 1 {
		t.Fatal("non-recheckable failure was hidden or sent another request")
	}
}
