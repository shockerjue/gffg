package ratelimit

import (
	"context"
	"time"
)

// Limiter defines the interface for rate limiting
type Limiter interface {
	// Allow checks if a request is allowed to proceed
	Allow(ctx context.Context, key string) (bool, error)

	// AllowN checks if N requests are allowed to proceed
	AllowN(ctx context.Context, key string, n int) (bool, error)

	// Wait blocks until a request is allowed to proceed
	Wait(ctx context.Context, key string) error

	// WaitN blocks until N requests are allowed to proceed
	WaitN(ctx context.Context, key string, n int) error

	// Reserve reserves a request slot and returns a Reservation
	Reserve(ctx context.Context, key string) (*Reservation, error)

	// ReserveN reserves N request slots and returns a Reservation
	ReserveN(ctx context.Context, key string, n int) (*Reservation, error)

	// UpdateLimit updates the rate limit for a key
	UpdateLimit(ctx context.Context, key string, limit Limit) error

	// GetLimit returns the current limit for a key
	GetLimit(ctx context.Context, key string) (Limit, error)

	// Close releases resources used by the limiter
	Close() error
}

// Limit represents a rate limit configuration
type Limit struct {
	// Rate is the number of requests per unit time
	Rate float64

	// Burst is the maximum burst size
	Burst int

	// Period is the time unit for the rate (e.g., time.Second)
	Period time.Duration

	// Algorithm specifies the rate limiting algorithm to use
	Algorithm Algorithm
}

// Algorithm defines the rate limiting algorithm type
type Algorithm string

const (
	// TokenBucket algorithm allows bursts up to burst size
	TokenBucket Algorithm = "token_bucket"

	// LeakyBucket algorithm smooths out request rate
	LeakyBucket Algorithm = "leaky_bucket"

	// FixedWindow algorithm uses fixed time windows
	FixedWindow Algorithm = "fixed_window"

	// SlidingWindow algorithm uses sliding time windows
	SlidingWindow Algorithm = "sliding_window"

	// Adaptive algorithm adjusts limits based on system load
	Adaptive Algorithm = "adaptive"
)

// Reservation represents a rate limit reservation
type Reservation struct {
	// OK indicates whether the reservation was successful
	OK bool

	// Key is the identifier for the rate limit
	Key string

	// Limit is the rate limit that was applied
	Limit Limit

	// Tokens is the number of tokens reserved
	Tokens int

	// TimeToAct is when the reservation can proceed
	TimeToAct time.Time

	// Cancel cancels the reservation
	Cancel func()
}

// Config holds rate limiter configuration
type Config struct {
	// DefaultLimit is the default rate limit applied when no specific limit is set
	DefaultLimit Limit

	// StorageBackend specifies where to store rate limit state
	StorageBackend StorageBackend

	// RedisConfig is used when StorageBackend is Redis
	RedisConfig *RedisConfig

	// LocalConfig is used when StorageBackend is Local
	LocalConfig *LocalConfig

	// CleanupInterval is how often to clean up expired rate limit data
	CleanupInterval time.Duration

	// MetricsEnabled enables rate limit metrics collection
	MetricsEnabled bool

	// DynamicLimits enables dynamic adjustment of rate limits
	DynamicLimits bool
}

// StorageBackend defines where rate limit state is stored
type StorageBackend string

const (
	// LocalStorage stores rate limit state in local memory
	LocalStorage StorageBackend = "local"

	// RedisStorage stores rate limit state in Redis
	RedisStorage StorageBackend = "redis"

	// ClusterStorage stores rate limit state in a distributed cluster
	ClusterStorage StorageBackend = "cluster"
)

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	// Addrs is a list of Redis addresses
	Addrs []string

	// Password is the Redis password
	Password string

	// DB is the Redis database number
	DB int

	// PoolSize is the connection pool size
	PoolSize int

	// MinIdleConns is the minimum number of idle connections
	MinIdleConns int

	// DialTimeout is the dial timeout
	DialTimeout time.Duration

	// ReadTimeout is the read timeout
	ReadTimeout time.Duration

	// WriteTimeout is the write timeout
	WriteTimeout time.Duration

	// PoolTimeout is the pool timeout
	PoolTimeout time.Duration
}

// LocalConfig holds local storage configuration
type LocalConfig struct {
	// MaxKeys is the maximum number of keys to store
	MaxKeys int

	// CleanupInterval is how often to clean up expired entries
	CleanupInterval time.Duration
}

// Metrics holds rate limit metrics
type Metrics struct {
	// Allowed counts allowed requests
	Allowed int64

	// Denied counts denied requests
	Denied int64

	// WaitTimeTotal is total wait time in nanoseconds
	WaitTimeTotal int64

	// LimitUpdates counts how many times limits were updated
	LimitUpdates int64

	// StorageErrors counts storage-related errors
	StorageErrors int64
}

// Error types
var (
	// ErrLimitExceeded is returned when rate limit is exceeded
	ErrLimitExceeded = &RateLimitError{Code: "LIMIT_EXCEEDED", Message: "rate limit exceeded"}

	// ErrInvalidLimit is returned when an invalid limit is provided
	ErrInvalidLimit = &RateLimitError{Code: "INVALID_LIMIT", Message: "invalid rate limit configuration"}

	// ErrStorage is returned when there's a storage error
	ErrStorage = &RateLimitError{Code: "STORAGE_ERROR", Message: "rate limit storage error"}

	// ErrTimeout is returned when a wait operation times out
	ErrTimeout = &RateLimitError{Code: "TIMEOUT", Message: "rate limit wait timeout"}

	// ErrCanceled is returned when a reservation is canceled
	ErrCanceled = &RateLimitError{Code: "CANCELED", Message: "rate limit reservation canceled"}
)

// RateLimitError represents a rate limiting error
type RateLimitError struct {
	Code    string
	Message string
	Key     string
	Limit   *Limit
	Err     error
}

func (e *RateLimitError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// NewRateLimitError creates a new RateLimitError
func NewRateLimitError(code, message, key string, limit *Limit, err error) *RateLimitError {
	return &RateLimitError{
		Code:    code,
		Message: message,
		Key:     key,
		Limit:   limit,
		Err:     err,
	}
}
