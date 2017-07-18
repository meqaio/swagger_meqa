package mqutil

import (
	"io"
	"log"
	"os"
)

func NewLogger(out io.Writer) *log.Logger {
	return log.New(out, "", (log.Ldate | log.Lmicroseconds | log.Lshortfile))
}

func NewStdLogger() *log.Logger {
	return NewLogger(os.Stderr)
}

// There is only one logger per process.
var Logger *log.Logger
