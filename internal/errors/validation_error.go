package errors

import "fmt"

// ValidationError represents an error from upstream API key validation,
// carrying both the HTTP status code and the parsed error message.
type ValidationError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("[status %d] %s", e.StatusCode, e.Message)
}
