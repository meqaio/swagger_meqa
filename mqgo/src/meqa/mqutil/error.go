package mqutil

import (
	"fmt"
	"runtime/debug"
)

const (
	ErrOK         = iota // 0
	ErrInvalid           // invalid parameters
	ErrNotFound          // resource not found
	ErrExpect            // the REST result doesn't match the expected value
	ErrHttp              // Http request failed
	ErrServerResp        // unexpected server response
	ErrInternal          // unexpected internal error (meqa error)
)

// Error implements MQ specific error type.
type Error interface {
	error
	Type() int
}

// TypedError holds a type and a back trace for easy debugging
type TypedError struct {
	errType int
	errMsg  string
}

func (e *TypedError) Error() string {
	return e.errMsg
}

func (e *TypedError) Type() int {
	return e.errType
}

func NewError(errType int, str string) error {
	buf := string(debug.Stack())
	err := TypedError{errType, ""}
	err.errMsg = fmt.Sprintf("==== %v ====\nError message:\n%s\nBacktrace:%v", errType, str, buf)
	return &err
}
