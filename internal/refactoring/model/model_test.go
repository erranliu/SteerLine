package model

import (
	"testing"
	"time"
)

func TestBudgetIsGateForNextRound(t *testing.T) {
	now := time.Now()
	value := "0.099999999999999999"
	r := Record{Input: Start{Budget: Budget{Dollars: "0.1", Minutes: "2", Rounds: 3}}, ActiveSince: &now, Operations: []Operation{{State: "complete", Cost: Cost{Amount: &value, Complete: true}}}}
	if r.BudgetExhausted(now.Add(time.Minute)) {
		t.Fatal("remaining budget cannot require reserve")
	}
	value = "0.1"
	if !r.BudgetExhausted(now) {
		t.Fatal("exact decimal threshold ignored")
	}
	r.Input.Budget = Budget{Rounds: 3}
	r.Round = 3
	if !r.BudgetExhausted(now) {
		t.Fatal("rounds ignored")
	}
	r.Input.Budget = Budget{Minutes: "2"}
	if !r.BudgetExhausted(now.Add(2 * time.Minute)) {
		t.Fatal("exact time threshold ignored")
	}
	r.Pause(now.Add(time.Minute))
	if r.Elapsed(now.Add(24*time.Hour)) != time.Minute {
		t.Fatal("paused human/offline time charged")
	}
	r.FinishRequested = true
	if !r.Active() {
		t.Fatal("budget must not cancel delivery")
	}
}
func TestBudgetValidation(t *testing.T) {
	for _, v := range []string{"0", "-1", "NaN", "Inf", "1e2", ".5", ""} {
		s := Start{Direction: "simplify", DirectionRevision: Digest([]byte("方向")), Profile: "local", Strategy: "cautious", Budget: Budget{Dollars: v}}
		if s.Validate() == nil {
			t.Fatal("invalid budget accepted", v)
		}
	}
}
