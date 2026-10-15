package sqlite

func recordFingerprint(value any) string { return operationDigest(mustJSON(value)) }
