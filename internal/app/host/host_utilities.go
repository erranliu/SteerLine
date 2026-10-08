package host

import (
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/pathguard"
)

func ptr[T any](value T) *T { return &value }

func now() time.Time { return time.Now().UTC() }

func duration(s string) time.Duration { value, _ := time.ParseDuration(s); return value }
func inside(root, path string) bool {
	return pathguard.Within(root, path)
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func clip(s string, n int) string {
	value := []rune(s)
	if len(value) > n {
		return string(value[:n]) + "…"
	}
	return s
}
func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
