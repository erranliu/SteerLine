package maintenance

import "testing"

func TestDecisionRoleBoundary(t *testing.T) {
	for _, v := range []struct {
		body, mode string
		valid      bool
	}{
		{`{"kind":"execute","prompt":"验证并修改","summary":"有限切片","question":""}`, "select", true},
		{`{"kind":"accept","prompt":"","summary":"证据充分","question":""}`, "audit", true},
		{`{"kind":"accept","prompt":"","summary":"","question":""}`, "select", false},
		{`{"kind":"finish","prompt":"","summary":"","question":""}`, "finish", false},
		{`{"kind":"execute","prompt":"","summary":"","question":""}`, "select", false},
		{`{"kind":"ask","prompt":"","summary":"","question":""}`, "audit", false},
		{`{"kind":"execute","prompt":"x","summary":"","question":"","tools":["write"]}`, "select", false},
	} {
		_, e := Accept([]byte(v.body), v.mode)
		if (e == nil) != v.valid {
			t.Fatal(v, e)
		}
	}
}
