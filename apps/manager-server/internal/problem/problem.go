package problem

import (
	"errors"
	"net/http"
	"strings"
)

const setupDocsBaseURL = "https://seakee.github.io/CPA-Manager-Plus/docs/troubleshooting/setup.html"

type Error struct {
	Code    string
	Message string
	Status  int
	DocsURL string
	Details map[string]any
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code string, message string, status int, docsAnchor string) *Error {
	return &Error{
		Code:    strings.TrimSpace(code),
		Message: strings.TrimSpace(message),
		Status:  normalizeStatus(status),
		DocsURL: setupDocsURL(docsAnchor),
	}
}

func Wrap(err error, code string, message string, status int, docsAnchor string) *Error {
	problem := New(code, message, status, docsAnchor)
	problem.Cause = err
	return problem
}

func WithDetails(err *Error, details map[string]any) *Error {
	if err != nil {
		err.Details = details
	}
	return err
}

func As(err error) (*Error, bool) {
	var target *Error
	if !errors.As(err, &target) || target == nil {
		return nil, false
	}
	return target, true
}

func setupDocsURL(anchor string) string {
	anchor = strings.TrimSpace(strings.TrimPrefix(anchor, "#"))
	if anchor == "" {
		return setupDocsBaseURL
	}
	return setupDocsBaseURL + "#" + anchor
}

func normalizeStatus(status int) int {
	if status >= 400 && status <= 599 {
		return status
	}
	return http.StatusInternalServerError
}
