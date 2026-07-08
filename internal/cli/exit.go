package cli

import "errors"

type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e ExitError) Unwrap() error {
	return e.Err
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit ExitError
	if errors.As(err, &exit) && exit.Code != 0 {
		return exit.Code
	}
	return 1
}
