package mqutil

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Test results constants
const (
	Passed         = "Passed"
	Failed         = "Failed"
	Skipped        = "Skipped"
	SchemaMismatch = "SchemaMismatch"
	Total          = "Total"
)

func NewLogger(out io.Writer) *log.Logger {
	Logger = log.New(out, "", (log.Ldate | log.Lmicroseconds | log.Lshortfile))
	return Logger
}

func NewStdLogger() *log.Logger {
	return NewLogger(os.Stdout)
}

func NewFileLogger(path string) *log.Logger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
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
