package executionconfig

import "testing"

func TestGQGenerateConcurrency(t *testing.T) {
	if n, err := GenerateConcurrency(nil); err != nil || n != 5 {
		t.Fatal(n, err)
	}
	for _, n := range []int{-1, 0, 1, 9} {
		got, err := GenerateConcurrency(&n)
		if n <= 0 && err == nil || n > 0 && (err != nil || got != n) {
			t.Fatal(n, got, err)
		}
	}
}
