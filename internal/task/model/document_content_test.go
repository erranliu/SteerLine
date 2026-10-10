package model

import "testing"

func TestAcceptanceHeadingLineEndings(t *testing.T) {
	for _, heading := range []string{"## Acceptance", "## 验收条件"} {
		for _, newline := range []string{"\n", "\r\n"} {
			if !HasAcceptanceHeading([]byte("# Spec" + newline + newline + heading + newline + "Check the result." + newline)) {
				t.Fatalf("valid heading rejected with line ending %q: %s", newline, heading)
			}
		}
	}
	if HasAcceptanceHeading([]byte("# Spec\r\nA sentence mentioning acceptance.\r\n")) {
		t.Fatal("prose became an acceptance heading")
	}
}
