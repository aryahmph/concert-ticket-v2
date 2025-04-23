package errs

import "fmt"

type HttpError struct {
	Code    int
	Message string
	Data    any
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("code %d: %s, data: %v", e.Code, e.Message, e.Data)
}
