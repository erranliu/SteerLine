package main

import "testing"

func TestWatchFlagsRouteTTYToStreamingMode(t *testing.T) {
	cases := []struct {
		name                                string
		json, raw, noTUI, follow, fromStart bool
		want                                bool
	}{
		{name: "default TTY viewer", follow: true, fromStart: true, want: true},
		{name: "follow false streams", follow: false, fromStart: true},
		{name: "from start false streams", follow: true, fromStart: false},
		{name: "JSON streams", json: true, follow: true, fromStart: true},
		{name: "raw streams", raw: true, follow: true, fromStart: true},
		{name: "no TUI streams", noTUI: true, follow: true, fromStart: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := useWatchTUI(true, tc.json, tc.raw, tc.noTUI, tc.follow, tc.fromStart); got != tc.want {
				t.Fatalf("useWatchTUI=%v want %v", got, tc.want)
			}
		})
	}
}
