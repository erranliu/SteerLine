package codexworker

// A command intent is not an app-server turn receipt.
func PendingIdentity(id string) (turnID, commandID string) { return "", id }
