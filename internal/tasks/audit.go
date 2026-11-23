package tasks

// DispatchAudit records delivery without creating agent-visible comment events.
func DispatchAudit(scope, id, message string) error {
	return withLock(scope, func() error { return appendEvents(scope, id, "system", []Event{{Kind: "dispatch", Message: message}}) })
}
