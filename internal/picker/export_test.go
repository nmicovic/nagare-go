package picker

// NewForTest returns a Model with the live tmux scanner disabled. Tests drive
// the session list exclusively via SessionsUpdatedMsg, which keeps runs
// hermetic (no dependency on the developer's current tmux state).
func NewForTest() Model {
	m := New()
	m.testNoScan = true
	return m
}
