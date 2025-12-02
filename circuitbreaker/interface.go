package circuitbreaker

import (
	"context"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed allows all requests through
	StateClosed State = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests to test if the service has recovered
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker defines the interface for circuit breaker pattern
type CircuitBreaker interface {
	// Execute runs the operation with circuit breaker protection
	Execute(ctx context.Context, operation func() (interface{}, error)) (interface{}, error)

	// AllowRequest checks if a request is allowed to proceed
	AllowRequest(ctx context.Context) bool

	// RecordSuccess records a successful operation
	RecordSuccess()

	// RecordFailure records a failed operation
	RecordFailure(err error)

	// GetState returns the current state of the circuit breaker
	GetState() State

	// GetMetrics returns circuit breaker metrics
	GetMetrics() Metrics

	// Reset resets the circuit breaker to closed state
	Reset()

	// Close releases resources used by the circuit breaker
	Close() error
}

// Config holds circuit breaker configuration
type Config struct {
	// Name identifies the circuit breaker
	Name string

	// ErrorThreshold is the percentage of errors that triggers the circuit to open
	ErrorThreshold float64

	// RequestThreshold is the minimum number of requests before the circuit can open
	RequestThreshold int64

	// SleepWindow is how long the circuit stays open before transitioning to half-open
	SleepWindow time.Duration

	// HalfOpenMaxRequests is the maximum number of requests allowed in half-open state
	HalfOpenMaxRequests int64

	// Timeout is the timeout for operations
	Timeout time.Duration

	// RollingWindow is the time window for metrics collection
	RollingWindow time.Duration

	// BucketCount is the number of buckets in the rolling window
	BucketCount int

	// FailurePredicate determines if an error should be counted as a failure
	FailurePredicate func(error) bool

	// OnStateChange is called when the circuit breaker state changes
	OnStateChange func(from, to State)

	// MetricsEnabled enables metrics collection
	MetricsEnabled bool
}

// Metrics holds circuit breaker metrics
type Metrics struct {
	// TotalRequests counts total requests
	TotalRequests int64

	// SuccessfulRequests counts successful requests
	SuccessfulRequests int64

	// FailedRequests counts failed requests
	FailedRequests int64

	// RejectedRequests counts rejected requests (circuit open)
	RejectedRequests int64

	// ErrorRate is the current error rate (0.0 to 1.0)
	ErrorRate float64

	// State is the current circuit breaker state
	State State

	// LastStateChange is when the state last changed
	LastStateChange time.Time

	// RollingWindowMetrics contains detailed rolling window metrics
	RollingWindowMetrics *RollingWindowMetrics
}

// RollingWindowMetrics holds rolling window metrics
type RollingWindowMetrics struct {
	// WindowSize is the size of the rolling window
	WindowSize time.Duration

	// BucketCount is the number of buckets
	BucketCount int

	// Buckets contains metrics for each time bucket
	Buckets []BucketMetrics

	// CurrentBucketIndex is the index of the current bucket
	CurrentBucketIndex int
}

// BucketMetrics holds metrics for a time bucket
type BucketMetrics struct {
	// StartTime is when the bucket started
	StartTime time.Time

	// TotalRequests counts requests in this bucket
	TotalRequests int64

	// SuccessfulRequests counts successful requests in this bucket
	SuccessfulRequests int64

	// FailedRequests counts failed requests in this bucket
	FailedRequests int64

	// ErrorRate is the error rate for this bucket
	ErrorRate float64
}

// Error types
var (
	// ErrCircuitOpen is returned when the circuit is open
	ErrCircuitOpen = &CircuitBreakerError{
		Code:    "CIRCUIT_OPEN",
		Message: "circuit breaker is open",
	}

	// ErrTimeout is returned when an operation times out
	ErrTimeout = &CircuitBreakerError{
		Code:    "TIMEOUT",
		Message: "operation timeout",
	}

	// ErrInvalidConfig is returned when configuration is invalid
	ErrInvalidConfig = &CircuitBreakerError{
		Code:    "INVALID_CONFIG",
		Message: "invalid circuit breaker configuration",
	}

	// ErrHalfOpenLimit is returned when half-open request limit is reached
	ErrHalfOpenLimit = &CircuitBreakerError{
		Code:    "HALF_OPEN_LIMIT",
		Message: "half-open request limit reached",
	}
)

// CircuitBreakerError represents a circuit breaker error
type CircuitBreakerError struct {
	Code    string
	Message string
	Name    string
	State   State
	Err     error
}

func (e *CircuitBreakerError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *CircuitBreakerError) Unwrap() error {
	return e.Err
}

// NewCircuitBreakerError creates a new CircuitBreakerError
func NewCircuitBreakerError(code, message, name string, state State, err error) *CircuitBreakerError {
	return &CircuitBreakerError{
		Code:    code,
		Message: message,
		Name:    name,
		State:   state,
		Err:     err,
	}
}

// IsCircuitOpenError checks if an error is a circuit open error
func IsCircuitOpenError(err error) bool {
	if cbErr, ok := err.(*CircuitBreakerError); ok {
		return cbErr.Code == "CIRCUIT_OPEN"
	}
	return false
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	if cbErr, ok := err.(*CircuitBreakerError); ok {
		return cbErr.Code == "TIMEOUT"
	}
	return false
}

// DefaultFailurePredicate is the default function to determine if an error is a failure
func DefaultFailurePredicate(err error) bool {
	return err != nil
}

// DefaultConfig returns a default circuit breaker configuration
func DefaultConfig(name string) Config {
	return Config{
		Name:                name,
		ErrorThreshold:      0.5, // 50% error rate
		RequestThreshold:    20,  // minimum 20 requests
		SleepWindow:         5 * time.Second,
		HalfOpenMaxRequests: 5,
		Timeout:             30 * time.Second,
		RollingWindow:       10 * time.Second,
		BucketCount:         10,
		FailurePredicate:    DefaultFailurePredicate,
		OnStateChange:       nil,
		MetricsEnabled:      true,
	}
}
