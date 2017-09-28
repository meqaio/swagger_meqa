package mqutil

import (
	"fmt"
	"io"
	"log"
	"os"
)

func NewLogger(out io.Writer) *log.Logger {
	return log.New(out, "", (log.Ldate | log.Lmicroseconds | log.Lshortfile))
}

func NewStdLogger() *log.Logger {
	return NewLogger(os.Stdout)
}

func NewFileLogger(path string) *log.Logger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Can't open %s, err: %s", path, err.Error())
		return nil
	}
	return NewLogger(f)
}

// There is only one logger per process.
var Logger *log.Logger

// Whether verbose mose is on
var Verbose bool
