package model

import (
	"reflect"
	"testing"
	"time"
)

func TestCycleIsDeterministicAndIncludesRepeatedEndpoint(t *testing.T) {
	cases := []struct {
		name  string
		edges []Edge
		want  []string
	}{
		{
			name: "smallest numeric cycle first",
			edges: []Edge{
				{Predecessor: "10", Successor: "11"},
				{Predecessor: "4", Successor: "10"},
				{Predecessor: "3", Successor: "2"},
				{Predecessor: "2", Successor: "3"},
			},
			want: []string{"2", "3", "2"},
		},
		{
			name:  "self cycle",
			edges: []Edge{{Predecessor: "7", Successor: "7"}},
			want:  []string{"7", "7"},
		},
		{name: "acyclic disconnected graph", edges: []Edge{{Predecessor: "10", Successor: "11"}, {Predecessor: "2", Successor: "3"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Cycle(tc.edges)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Cycle(%v) = %v, want %v", tc.edges, got, tc.want)
			}
			for left, right := 0, len(tc.edges)-1; left < right; left, right = left+1, right-1 {
				tc.edges[left], tc.edges[right] = tc.edges[right], tc.edges[left]
			}
			if reversed := Cycle(tc.edges); !reflect.DeepEqual(reversed, tc.want) {
				t.Fatalf("Cycle(reversed edges) = %v, want %v", reversed, tc.want)
			}
		})
	}
}

func TestStableTaskOrderUsesPriorityTimeAndNumericID(t *testing.T) {
	base := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	nodes := []Node{
		{ID: "9", Priority: 2, CreatedAt: base.Add(time.Second)},
		{ID: "10", Priority: 1, CreatedAt: base},
		{ID: "11", Priority: 1, CreatedAt: base},
		{ID: "12", Priority: 0, CreatedAt: base},
		{ID: "2", Priority: 1, CreatedAt: base},
	}
	edges := []Edge{{Predecessor: "9", Successor: "12"}}
	want := []string{"2", "10", "11", "9", "12"}
	if got := StableTaskOrder(nodes, edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("StableTaskOrder() = %v, want %v", got, want)
	}
}
