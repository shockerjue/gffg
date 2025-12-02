package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

// tokenBucket implements the token bucket rate limiting algorithm
type tokenBucket struct {
	mu sync.RWMutex

	// Configuration
	config *Config
	limits map[string]*bucketLimit

	// Metrics
	metrics *Metrics
	enabled bool

	// Cleanup
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// bucketLimit represents a token bucket for a specific key
type bucketLimit struct {
	limit      Limit
	tokens     float64
	lastUpdate time.Time
	createdAt  time.Time
	lastAccess time.Time
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(config *Config) (Limiter, error) {
	if config == nil {
		config = &Config{
			DefaultLimit: Limit{
				Rate:      100,
				Burst:     200,
				Period:    time.Second,
				Algorithm: TokenBucket,
			},
			StorageBackend: LocalStorage,
			LocalConfig: &LocalConfig{
				MaxKeys:         10000,
				CleanupInterval: 5 * time.Minute,
			},
			CleanupInterval: 5 * time.Minute,
			MetricsEnabled:  true,
			DynamicLimits:   false,
		}
	}

	if config.DefaultLimit.Period <= 0 {
		config.DefaultLimit.Period = time.Second
	}

	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	tb := &tokenBucket{
		config:   config,
		limits:   make(map[string]*bucketLimit),
		metrics:  &Metrics{},
		enabled:  config.MetricsEnabled,
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	tb.cleanupTicker = time.NewTicker(config.CleanupInterval)
	go tb.cleanup()

	return tb, nil
}

// Allow checks if a request is allowed to proceed
func (tb *tokenBucket) Allow(ctx context.Context, key string) (bool, error) {
	return tb.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed to proceed
func (tb *tokenBucket) AllowN(ctx context.Context, key string, n int) (bool, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Get or create bucket limit
	bucket := tb.getOrCreateBucket(key)

	// Update tokens based on elapsed time
	tb.updateTokens(bucket)

	// Check if we have enough tokens
	if bucket.tokens >= float64(n) {
		bucket.tokens -= float64(n)
		bucket.lastAccess = time.Now()

		if tb.enabled {
			tb.metrics.Allowed++
		}

		return true, nil
	}

	if tb.enabled {
		tb.metrics.Denied++
	}

	return false, NewRateLimitError(
		"LIMIT_EXCEEDED",
		"rate limit exceeded",
		key,
		&bucket.limit,
		nil,
	)
}

// Wait blocks until a request is allowed to proceed
func (tb *tokenBucket) Wait(ctx context.Context, key string) error {
	return tb.WaitN(ctx, key, 1)
}

// WaitN blocks until N requests are allowed to proceed
func (tb *tokenBucket) WaitN(ctx context.Context, key string, n int) error {
	start := time.Now()
	defer func() {
		if tb.enabled {
			tb.metrics.WaitTimeTotal += time.Since(start).Nanoseconds()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return NewRateLimitError(
				"CANCELED",
				"context canceled",
				key,
				nil,
				ctx.Err(),
			)
		default:
			allowed, err := tb.AllowN(ctx, key, n)
			if err != nil {
				if rle, ok := err.(*RateLimitError); ok && rle.Code == "LIMIT_EXCEEDED" {
					// Calculate wait time
					bucket := tb.getBucket(key)
					if bucket == nil {
						time.Sleep(10 * time.Millisecond)
						continue
					}

					// Calculate time needed to get enough tokens
					tokensNeeded := float64(n) - bucket.tokens
					refillRate := bucket.limit.Rate / float64(bucket.limit.Period/time.Second)
					waitTime := time.Duration(tokensNeeded/refillRate*float64(time.Second)) * time.Second

					// Wait for tokens to refill or context timeout
					timer := time.NewTimer(waitTime)
					select {
					case <-timer.C:
						continue
					case <-ctx.Done():
						timer.Stop()
						return NewRateLimitError(
							"CANCELED",
							"context canceled while waiting",
							key,
							&bucket.limit,
							ctx.Err(),
						)
					}
				}
				return err
			}

			if allowed {
				return nil
			}
		}
	}
}

// Reserve reserves a request slot and returns a Reservation
func (tb *tokenBucket) Reserve(ctx context.Context, key string) (*Reservation, error) {
	return tb.ReserveN(ctx, key, 1)
}

// ReserveN reserves N request slots and returns a Reservation
func (tb *tokenBucket) ReserveN(ctx context.Context, key string, n int) (*Reservation, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Get or create bucket limit
	bucket := tb.getOrCreateBucket(key)

	// Update tokens based on elapsed time
	tb.updateTokens(bucket)

	// Check if we have enough tokens
	if bucket.tokens >= float64(n) {
		bucket.tokens -= float64(n)
		bucket.lastAccess = time.Now()

		if tb.enabled {
			tb.metrics.Allowed++
		}

		reservation := &Reservation{
			OK:        true,
			Key:       key,
			Limit:     bucket.limit,
			Tokens:    n,
			TimeToAct: time.Now(),
			Cancel: func() {
				tb.mu.Lock()
				defer tb.mu.Unlock()
				bucket.tokens += float64(n)
			},
		}

		return reservation, nil
	}

	if tb.enabled {
		tb.metrics.Denied++
	}

	// Calculate when enough tokens will be available
	tokensNeeded := float64(n) - bucket.tokens
	refillRate := bucket.limit.Rate / float64(bucket.limit.Period/time.Second)
	waitTime := time.Duration(tokensNeeded/refillRate*float64(time.Second)) * time.Second

	reservation := &Reservation{
		OK:        false,
		Key:       key,
		Limit:     bucket.limit,
		Tokens:    n,
		TimeToAct: time.Now().Add(waitTime),
		Cancel:    func() {}, // No-op for denied reservations
	}

	return reservation, NewRateLimitError(
		"LIMIT_EXCEEDED",
		"rate limit exceeded",
		key,
		&bucket.limit,
		nil,
	)
}

// UpdateLimit updates the rate limit for a key
func (tb *tokenBucket) UpdateLimit(ctx context.Context, key string, limit Limit) error {
	if limit.Rate <= 0 {
		return ErrInvalidLimit
	}
	if limit.Burst <= 0 {
		return ErrInvalidLimit
	}
	if limit.Period <= 0 {
		limit.Period = time.Second
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	bucket := tb.getOrCreateBucket(key)
	bucket.limit = limit

	// Reset tokens to burst size if new burst is smaller
	if bucket.tokens > float64(limit.Burst) {
		bucket.tokens = float64(limit.Burst)
	}

	bucket.lastUpdate = time.Now()

	if tb.enabled {
		tb.metrics.LimitUpdates++
	}

	return nil
}

// GetLimit returns the current limit for a key
func (tb *tokenBucket) GetLimit(ctx context.Context, key string) (Limit, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	bucket := tb.getBucket(key)
	if bucket == nil {
		return tb.config.DefaultLimit, nil
	}

	return bucket.limit, nil
}

// Close releases resources used by the limiter
func (tb *tokenBucket) Close() error {
	close(tb.stopChan)
	tb.cleanupTicker.Stop()
	return nil
}

// getOrCreateBucket gets or creates a bucket for the given key
func (tb *tokenBucket) getOrCreateBucket(key string) *bucketLimit {
	bucket, exists := tb.limits[key]
	if !exists {
		bucket = &bucketLimit{
			limit:      tb.config.DefaultLimit,
			tokens:     float64(tb.config.DefaultLimit.Burst),
			lastUpdate: time.Now(),
			createdAt:  time.Now(),
			lastAccess: time.Now(),
		}
		tb.limits[key] = bucket
	}
	return bucket
}

// getBucket gets a bucket for the given key
func (tb *tokenBucket) getBucket(key string) *bucketLimit {
	return tb.limits[key]
}

// updateTokens updates the token count based on elapsed time
func (tb *tokenBucket) updateTokens(bucket *bucketLimit) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdate)

	// Calculate tokens to add based on elapsed time
	refillRate := bucket.limit.Rate / float64(bucket.limit.Period/time.Second)
	tokensToAdd := refillRate * elapsed.Seconds()

	// Add tokens, but don't exceed burst size
	bucket.tokens += tokensToAdd
	if bucket.tokens > float64(bucket.limit.Burst) {
		bucket.tokens = float64(bucket.limit.Burst)
	}

	bucket.lastUpdate = now
}

// cleanup removes old buckets to prevent memory leaks
func (tb *tokenBucket) cleanup() {
	for {
		select {
		case <-tb.stopChan:
			return
		case <-tb.cleanupTicker.C:
			tb.mu.Lock()
			now := time.Now()
			expiredKeys := make([]string, 0)

			for key, bucket := range tb.limits {
				// Remove buckets that haven't been accessed in 2x cleanup interval
				if now.Sub(bucket.lastAccess) > 2*tb.config.CleanupInterval {
					expiredKeys = append(expiredKeys, key)
				}
			}

			for _, key := range expiredKeys {
				delete(tb.limits, key)
			}

			if len(expiredKeys) > 0 && zzlog.IsDebugEnabled() {
				zzlog.Debugw("token bucket cleanup",
					zap.Int("removed_buckets", len(expiredKeys)),
					zap.Int("remaining_buckets", len(tb.limits)))
			}

			tb.mu.Unlock()
		}
	}
}

// GetMetrics returns the current metrics
func (tb *tokenBucket) GetMetrics() *Metrics {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	return &Metrics{
		Allowed:       tb.metrics.Allowed,
		Denied:        tb.metrics.Denied,
		WaitTimeTotal: tb.metrics.WaitTimeTotal,
		LimitUpdates:  tb.metrics.LimitUpdates,
		StorageErrors: tb.metrics.StorageErrors,
	}
}
