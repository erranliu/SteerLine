package model

import "regexp"

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }

func MaterialDirectory(key string) string {
	if !ValidKey(key) {
		return ""
	}
	return "SteerlineFlow/environments/" + key
}

func materialPath(key, name string) string {
	if !ValidKey(key) {
		return ""
	}
	return MaterialDirectory(key) + "/" + name
}

func DocumentPath(key string) string { return materialPath(key, "environment.md") }
func ScriptPath(key, extension string) string {
	if extension != "ps1" && extension != "sh" {
		return ""
	}
	return materialPath(key, "verify-environment."+extension)
}
