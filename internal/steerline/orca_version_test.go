package steerline

import "testing"

func TestOrcaVersionFamilyUsesMajorMinorOnly(t *testing.T) {
	for _, pair := range [][2]string{
		{"1.4.209", "1.4.210"},
		{"v1.4.1", "1.4.999"},
	} {
		if !sameOrcaVersionFamily(pair[0], pair[1]) {
			t.Errorf("versions %q and %q should share a family", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{
		{"1.4.209", "1.5.0"},
		{"1.4.209", "2.4.209"},
		{"unknown", "1.4.210"},
	} {
		if sameOrcaVersionFamily(pair[0], pair[1]) {
			t.Errorf("versions %q and %q must not share a family", pair[0], pair[1])
		}
	}
}
