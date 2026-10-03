package console

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func decisionSummary(r *contracts.Run) string {
	if r.Decision == nil {
		return ""
	}
	if r.Decision.OptionID != "" {
		for _, o := range r.Decision.Request.Options {
			if o.ID == r.Decision.OptionID {
				if r.Decision.Note != "" {
					return r.Decision.OptionID + ": " + o.Label + " — " + r.Decision.Note
				}
				return r.Decision.OptionID + ": " + o.Label
			}
		}
		return r.Decision.OptionID
	}
	return r.Decision.Note
}

func viewCanonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
