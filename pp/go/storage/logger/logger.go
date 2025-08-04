package logger

func noop(string, ...any) {}

// These variables are set by the common log package.
var (
	Errorf = noop
	Warnf  = noop
	Infof  = noop
	Debugf = noop
)

// Unset logger funcs to NoOp
func Unset() {
	Errorf = noop
	Warnf = noop
	Infof = noop
	Debugf = noop
}
