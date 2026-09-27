package steerline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var orcaVersionPattern = regexp.MustCompile(`(?:^|[^0-9])v?([0-9]+)\.([0-9]+)(?:\.[0-9]+)?`)

// orcaVersionFamily returns the user-selected compatibility key. Full versions
// remain in capability records; only the major.minor prefix is compared for
// the explicit Orca compatibility policy.
func orcaVersionFamily(version string) (string, bool) {
	match := orcaVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if len(match) != 3 {
		return "", false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return "", false
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%d.%d", major, minor), true
}

func sameOrcaVersionFamily(a, b string) bool {
	aFamily, aOK := orcaVersionFamily(a)
	bFamily, bOK := orcaVersionFamily(b)
	return aOK && bOK && aFamily == bFamily
}
