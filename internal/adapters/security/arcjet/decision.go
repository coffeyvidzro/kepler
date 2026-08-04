package arcjet

// Mode records whether an adapter decision is enforced or observed.
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeObserve Mode = "observe"
)
