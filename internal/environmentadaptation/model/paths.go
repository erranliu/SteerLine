package model

import "regexp"

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }

func DocumentPath() string { return "SteerlineFlow/environment.md" }
func ScriptPath(extension string) string {
	if extension != "ps1" && extension != "sh" {
		return ""
	}
	return "SteerlineFlow/scripts/verify-environment." + extension
}
