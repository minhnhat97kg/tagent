package tasks

// DispatchAudit records delivery without creating agent-visible comment events.
func DispatchAudit(scope, id, message string) error {
	return withLock(scope, func() error { return appendEvents(scope, id, "system", []Event{{Kind: "dispatch", Message: message}}) })
}

// RecordDispatch stores what was actually sent: role, harness, model, pane and
// the rendered prompt. Old logs have no such events; readers just skip them.
func RecordDispatch(scope, id string, e Event) error {
	e.Kind = "dispatch"
	return withLock(scope, func() error { return appendEvents(scope, id, "system", []Event{e}) })
}
