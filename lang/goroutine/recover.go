package goroutine

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/fino-io/finokit/logs"
)

func Recover(ctx context.Context, errPtr *error) {
	e := recover()
	if e == nil {
		return
	}

	var _err error
	if errPtr != nil && *errPtr != nil {
		_err = fmt.Errorf("panic: %v, origin error: %v", e, *errPtr)
	} else {
		_err = fmt.Errorf("panic: %v", e)
	}

	if errPtr != nil {
		*errPtr = _err
	}

	logs.Errorf("panic occurred, error: %v\nstacktrace:\n%s", fmt.Errorf("%v", e), debug.Stack())
}

func Recovery(ctx context.Context) {
	e := recover()
	if e == nil {
		return
	}

	logs.Errorf("panic occurred, error: %v\n stacktrace:\n%s", fmt.Errorf("%v", e), debug.Stack())
}
