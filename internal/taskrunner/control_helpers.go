package taskrunner

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

const ControlProtocolVersion = 1

var controlIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var controlDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validControlDigest(value string) bool { return controlDigestPattern.MatchString(value) }
func canonicalControlPath(value string) string {
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}
func decodeControlJSON(body []byte, value any) error {
	return b.DecodeStrictJSON(body, value, "expected one control payload")
}
func RequestDigest(request api.ControlRequest) string { return digest(request) }
