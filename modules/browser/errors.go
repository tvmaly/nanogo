package browser

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeNotFound            ErrorCode = "not_found"
	CodeNotAllowed          ErrorCode = "not_allowed"
	CodeNotAuthorized       ErrorCode = "not_authorized"
	CodeTimeout             ErrorCode = "timeout"
	CodeAdapterUnavailable  ErrorCode = "adapter_unavailable"
	CodeBrowserCrashed      ErrorCode = "browser_crashed"
	CodeStaleRef            ErrorCode = "stale_ref"
	CodeCrossOriginBlocked  ErrorCode = "cross_origin_blocked"
	CodePolicyDenied        ErrorCode = "policy_denied"
	CodeInvalidRequest      ErrorCode = "invalid_request"
	CodeUnsupportedVersion  ErrorCode = "unsupported_version"
	CodeUnsupportedStrategy ErrorCode = "unsupported_strategy"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Reason  string    `json:"reason,omitempty"`
}

func (e *Error) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func E(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

func PolicyDenied(reason, message string) *Error {
	return &Error{Code: CodePolicyDenied, Reason: reason, Message: message}
}

func Invalid(message string) *Error {
	return E(CodeInvalidRequest, message)
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	return &Error{Code: CodeInvalidRequest, Message: err.Error()}
}
