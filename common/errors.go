package common

import (
	"fmt"
)

// Error codes for RPC framework
const (
	ErrCodeInternal      = 500
	ErrCodeTimeout       = 408
	ErrCodeNotFound      = 404
	ErrCodeRateLimit     = 429
	ErrCodeUnauthorized  = 401
	ErrCodeBadRequest    = 400
	ErrCodeNotRegistered = 405
	ErrCodeCallFailed    = 505
)

// RPCError represents a structured error in the RPC framework
type RPCError struct {
	Code    int32
	Message string
	TraceID string
	Err     error
}

// Error implements the error interface
func (e *RPCError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("RPCError[code=%d, trace=%s]: %s - %v", e.Code, e.TraceID, e.Message, e.Err)
	}
	return fmt.Sprintf("RPCError[code=%d, trace=%s]: %s", e.Code, e.TraceID, e.Message)
}

// Unwrap returns the underlying error
func (e *RPCError) Unwrap() error {
	return e.Err
}

// NewRPCError creates a new RPCError
func NewRPCError(code int32, message, traceID string) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		TraceID: traceID,
	}
}

// NewRPCErrorWithErr creates a new RPCError with underlying error
func NewRPCErrorWithErr(code int32, message, traceID string, err error) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		TraceID: traceID,
		Err:     err,
	}
}

// WrapError wraps an existing error as RPCError
func WrapError(err error, code int32, message, traceID string) *RPCError {
	if rpcErr, ok := err.(*RPCError); ok {
		return rpcErr
	}
	return &RPCError{
		Code:    code,
		Message: message,
		TraceID: traceID,
		Err:     err,
	}
}

// IsTimeoutError checks if error is a timeout error
func IsTimeoutError(err error) bool {
	if rpcErr, ok := err.(*RPCError); ok {
		return rpcErr.Code == ErrCodeTimeout
	}
	return false
}

// IsRateLimitError checks if error is a rate limit error
func IsRateLimitError(err error) bool {
	if rpcErr, ok := err.(*RPCError); ok {
		return rpcErr.Code == ErrCodeRateLimit
	}
	return false
}

// Error helper functions

// ErrRPCNotFound returns a not found error
func ErrRPCNotFound(rpcID uint64, traceID string) *RPCError {
	return NewRPCError(ErrCodeNotFound, fmt.Sprintf("RPC method not found: rid=%d", rpcID), traceID)
}

// ErrRPCTimeout returns a timeout error
func ErrRPCTimeout(traceID string) *RPCError {
	return NewRPCError(ErrCodeTimeout, "RPC call timeout", traceID)
}

// ErrRPCRateLimit returns a rate limit error
func ErrRPCRateLimit(method, traceID string) *RPCError {
	return NewRPCError(ErrCodeRateLimit, fmt.Sprintf("Rate limit exceeded for method: %s", method), traceID)
}

// ErrRPCInternal returns an internal server error
func ErrRPCInternal(traceID string, err error) *RPCError {
	return NewRPCErrorWithErr(ErrCodeInternal, "Internal server error", traceID, err)
}

// ErrRPCCallFailed returns a call failed error
func ErrRPCCallFailed(method, traceID string, err error) *RPCError {
	return NewRPCErrorWithErr(ErrCodeCallFailed, fmt.Sprintf("RPC call failed: %s", method), traceID, err)
}

// ErrRPCNotRegistered returns a not registered error
func ErrRPCNotRegistered(rpcID uint64, traceID string) *RPCError {
	return NewRPCError(ErrCodeNotRegistered, fmt.Sprintf("RPC method not registered: rid=%d", rpcID), traceID)
}

// ErrRPCBadRequest returns a bad request error
func ErrRPCBadRequest(message, traceID string) *RPCError {
	return NewRPCError(ErrCodeBadRequest, message, traceID)
}

// ErrRPCMarshal returns a marshal error
func ErrRPCMarshal(traceID string, err error) *RPCError {
	return NewRPCErrorWithErr(ErrCodeBadRequest, "Failed to marshal request", traceID, err)
}

// ErrRPCUnmarshal returns an unmarshal error
func ErrRPCUnmarshal(traceID string, err error) *RPCError {
	return NewRPCErrorWithErr(ErrCodeBadRequest, "Failed to unmarshal request", traceID, err)
}
