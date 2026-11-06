package primitive

import "fmt"

// LogLevel is the highest level that Log prints; 0 keeps the program silent.
var LogLevel int

// Log prints to standard output when level is at most LogLevel.
func Log(level int, format string, a ...interface{}) {
	if LogLevel >= level {
		fmt.Printf(format, a...)
	}
}

func vv(format string, a ...interface{}) {
	Log(2, "  "+format, a...)
}
