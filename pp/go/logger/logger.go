package logger

// noop is a no-op logger function.
func noop(string, ...any) {}

// These variables are set by the common log package.
var (
	Errorf        = noop
	Warnf         = noop
	Infof         = noop
	Debugf        = noop
	Log    Logger = noopLogger{}
)

// Unset logger funcs to NoOp.
func Unset() {
	Errorf = noop
	Warnf = noop
	Infof = noop
	Debugf = noop
	Log = noopLogger{}
}

//
// Logger
//

// Logger is a logger interface.
type Logger interface {
	Log(keyvals ...any) error
}

//
// noopLogger
//

// noopLogger is a no-op logger.
type noopLogger struct{}

// Log implements the [Logger] interface.
func (noopLogger) Log(...any) error { return nil }
