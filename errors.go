package intosql

import (
	"errors"
	"fmt"
)

// Error is an answer from the IntoSQL API that wasn't a success. Code doesn't change between
// versions; branch on it with errors.As.
type Error struct {
	// The HTTP status.
	Status int
	// Stable, such as "over_quota"; "http_error" when the answer had no error body.
	Code string
	// For people; it can change.
	Message string
	// Per field messages when Code is "invalid_request"; otherwise empty.
	Fields map[string][]string
	// The answer's X-Request-Id; quote it when you write to IntoSQL. Empty when there was none.
	RequestID string
}

func (e *Error) Error() string {
	return e.Message
}

// ConversionFailedError is a conversion that ended as failed, with the stable code and the
// engine's problems.
type ConversionFailedError struct {
	ID       string
	Code     string
	Problems []Problem
	Message  string
}

func newConversionFailedError(id string, failure *ConversionFailure) *ConversionFailedError {
	err := &ConversionFailedError{
		ID:       id,
		Code:     "internal_error",
		Problems: []Problem{},
		Message:  fmt.Sprintf("Conversion %s failed.", id),
	}

	if failure != nil {
		err.Code = failure.Code
		err.Message = failure.Message

		if failure.Problems != nil {
			err.Problems = failure.Problems
		}
	}

	return err
}

func (e *ConversionFailedError) Error() string {
	return e.Message
}

// ConversionCanceledError is a conversion that was canceled while it was awaited.
type ConversionCanceledError struct {
	ID string
}

func (e *ConversionCanceledError) Error() string {
	return fmt.Sprintf("Conversion %s was canceled.", e.ID)
}

// WaitTimeoutError is returned when waiting goes past its timeout. The conversion keeps running;
// read it later by its ID.
type WaitTimeoutError struct {
	ID     string
	Status ConversionStatus
}

func (e *WaitTimeoutError) Error() string {
	return fmt.Sprintf("Conversion %s is still %s.", e.ID, e.Status)
}

// ErrInvalidSignature is returned by ParseWebhook when the IntoSQL-Signature header doesn't match
// the body, is malformed, or is too old.
var ErrInvalidSignature = errors.New("intosql: the IntoSQL-Signature does not match this body")
