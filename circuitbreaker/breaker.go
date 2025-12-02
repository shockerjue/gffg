package circuitbreaker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

// breaker implements the CircuitBreaker interface
type breaker struct {
	mu sync.RWMutex

	// Configuration
	config Config

	// State
	state         State
	lastStateTime time.Time
	halfOpenCount int64

	// Metrics
	rollingWindow *rollingWindow
	metrics       Metrics

	// Concurrency control
	concurrentRequests int64
	maxConcurrent      int64

	// Callbacks
	onStateChange func(from, to State)

	// Stop channel for cleanup
	stopChan chan struct{}
}

// rollingWindow implements a rolling window for metrics collection
type rollingWindow struct {
	windowSize   time.Duration
	bucketCount  int
	bucketSize   time.Duration
	buckets      []BucketMetrics
	currentIndex int
	lastRotate   time.Time
	mu           sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config Config) (CircuitBreaker, error) {
	// Validate configuration
	if config.ErrorThreshold <= 0 || config.ErrorThreshold > 1 {
		return nil, ErrInvalidConfig
	}
	if config.RequestThreshold <= 0 {
		return nil, ErrInvalidConfig
	}
	if config.SleepWindow <= 0 {
		config.SleepWindow = 5 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.RollingWindow <= 0 {
		config.RollingWindow = 10 * time.Second
	}
	if config.BucketCount <= 0 {
		config.BucketCount = 10
	}
	if config.FailurePredicate == nil {
		config.FailurePredicate = DefaultFailurePredicate
	}

	// Create rolling window
	rw := &rollingWindow{
		windowSize:   config.RollingWindow,
		bucketCount:  config.BucketCount,
		bucketSize:   config.RollingWindow / time.Duration(config.BucketCount),
		buckets:      make([]BucketMetrics, config.BucketCount),
		currentIndex: 0,
		lastRotate:   time.Now(),
	}

	// Initialize buckets
	for i := range rw.buckets {
		rw.buckets[i] = BucketMetrics{
			StartTime: time.Now().Add(-time.Duration(i) * rw.bucketSize),
		}
	}

	cb := &breaker{
		config:        config,
		state:         StateClosed,
		lastStateTime: time.Now(),
		rollingWindow: rw,
		metrics: Metrics{
			State:           StateClosed,
			LastStateChange: time.Now(),
			RollingWindowMetrics: &RollingWindowMetrics{
				WindowSize:         config.RollingWindow,
				BucketCount:        config.BucketCount,
				Buckets:            make([]BucketMetrics, config.BucketCount),
				CurrentBucketIndex: 0,
			},
		},
		maxConcurrent: 1000, // Default max concurrent requests
		onStateChange: config.OnStateChange,
		stopChan:      make(chan struct{}),
	}

	// Copy initial buckets to metrics
	copy(cb.metrics.RollingWindowMetrics.Buckets, rw.buckets)

	// Start background goroutine for state management
	go cb.stateManager()

	return cb, nil
}

// Execute runs the operation with circuit breaker protection
func (cb *breaker) Execute(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
	// Check if request is allowed
	if !cb.AllowRequest(ctx) {
		atomic.AddInt64(&cb.metrics.RejectedRequests, 1)
		return nil, ErrCircuitOpen
	}

	// Increment concurrent request counter
	atomic.AddInt64(&cb.concurrentRequests, 1)
	defer atomic.AddInt64(&cb.concurrentRequests, -1)

	// Create context with timeout
	if cb.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.config.Timeout)
		defer cancel()
	}

	// Execute operation
	startTime := time.Now()
	result, err := operation()
	duration := time.Since(startTime)

	// Record result
	if err != nil && cb.config.FailurePredicate(err) {
		cb.RecordFailure(err)
	} else {
		cb.RecordSuccess()
	}

	// Log slow operations
	if duration > 1*time.Second && zzlog.IsDebugEnabled() {
		zzlog.Debugw("circuit breaker slow operation",
			zap.String("name", cb.config.Name),
			zap.Duration("duration", duration),
			zap.String("state", cb.state.String()),
			zap.Error(err))
	}

	return result, err
}

// AllowRequest checks if a request is allowed to proceed
func (cb *breaker) AllowRequest(ctx context.Context) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Update metrics
	atomic.AddInt64(&cb.metrics.TotalRequests, 1)

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if sleep window has passed
		if time.Since(cb.lastStateTime) >= cb.config.SleepWindow {
			cb.mu.RUnlock()
			cb.transitionToHalfOpen()
			cb.mu.RLock()
			return cb.state == StateHalfOpen
		}
		return false

	case StateHalfOpen:
		// Check if we've reached the half-open request limit
		if atomic.LoadInt64(&cb.halfOpenCount) >= cb.config.HalfOpenMaxRequests {
			return false
		}
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *breaker) RecordSuccess() {
	cb.rollingWindow.record(true)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Update metrics
	atomic.AddInt64(&cb.metrics.SuccessfulRequests, 1)

	// State-specific logic
	switch cb.state {
	case StateHalfOpen:
		// If we get enough successes in half-open state, close the circuit
		successCount := atomic.LoadInt64(&cb.halfOpenCount)
		if successCount >= cb.config.HalfOpenMaxRequests/2 {
			cb.transitionToClosed()
		}
	}
}

// RecordFailure records a failed operation
func (cb *breaker) RecordFailure(err error) {
	cb.rollingWindow.record(false)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Update metrics
	atomic.AddInt64(&cb.metrics.FailedRequests, 1)

	// Calculate error rate
	errorRate := cb.calculateErrorRate()

	// Update metrics error rate
	cb.metrics.ErrorRate = errorRate

	// State-specific logic
	switch cb.state {
	case StateClosed:
		// Check if we should open the circuit
		totalRequests := cb.metrics.SuccessfulRequests + cb.metrics.FailedRequests
		if totalRequests >= cb.config.RequestThreshold && errorRate >= cb.config.ErrorThreshold {
			cb.transitionToOpen()
		}

	case StateHalfOpen:
		// Any failure in half-open state should open the circuit
		cb.transitionToOpen()
	}

	// Log failure if debug enabled
	if zzlog.IsDebugEnabled() {
		zzlog.Debugw("circuit breaker recorded failure",
			zap.String("name", cb.config.Name),
			zap.String("state", cb.state.String()),
			zap.Float64("error_rate", errorRate),
			zap.Error(err))
	}
}

// GetState returns the current state of the circuit breaker
func (cb *breaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetMetrics returns circuit breaker metrics
func (cb *breaker) GetMetrics() Metrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Calculate current error rate
	errorRate := cb.calculateErrorRate()

	// Copy metrics
	metrics := cb.metrics
	metrics.ErrorRate = errorRate
	metrics.State = cb.state

	// Get rolling window metrics
	rwMetrics := cb.rollingWindow.getMetrics()
	metrics.RollingWindowMetrics = rwMetrics

	return metrics
}

// Reset resets the circuit breaker to closed state
func (cb *breaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionToClosed()
	cb.rollingWindow.reset()

	// Reset metrics
	cb.metrics = Metrics{
		State:           StateClosed,
		LastStateChange: time.Now(),
		RollingWindowMetrics: &RollingWindowMetrics{
			WindowSize:         cb.config.RollingWindow,
			BucketCount:        cb.config.BucketCount,
			Buckets:            make([]BucketMetrics, cb.config.BucketCount),
			CurrentBucketIndex: 0,
		},
	}

	atomic.StoreInt64(&cb.halfOpenCount, 0)

	zzlog.Infow("circuit breaker reset",
		zap.String("name", cb.config.Name))
}

// Close releases resources used by the circuit breaker
func (cb *breaker) Close() error {
	close(cb.stopChan)
	return nil
}

// transitionToClosed transitions the circuit breaker to closed state
func (cb *breaker) transitionToClosed() {
	if cb.state == StateClosed {
		return
	}

	from := cb.state
	cb.state = StateClosed
	cb.lastStateTime = time.Now()
	atomic.StoreInt64(&cb.halfOpenCount, 0)

	cb.metrics.State = StateClosed
	cb.metrics.LastStateChange = time.Now()

	zzlog.Infow("circuit breaker closed",
		zap.String("name", cb.config.Name),
		zap.String("from", from.String()))

	if cb.onStateChange != nil {
		cb.onStateChange(from, StateClosed)
	}
}

// transitionToOpen transitions the circuit breaker to open state
func (cb *breaker) transitionToOpen() {
	if cb.state == StateOpen {
		return
	}

	from := cb.state
	cb.state = StateOpen
	cb.lastStateTime = time.Now()
	atomic.StoreInt64(&cb.halfOpenCount, 0)

	cb.metrics.State = StateOpen
	cb.metrics.LastStateChange = time.Now()

	zzlog.Warnw("circuit breaker opened",
		zap.String("name", cb.config.Name),
		zap.String("from", from.String()),
		zap.Float64("error_rate", cb.metrics.ErrorRate))

	if cb.onStateChange != nil {
		cb.onStateChange(from, StateOpen)
	}
}

// transitionToHalfOpen transitions the circuit breaker to half-open state
func (cb *breaker) transitionToHalfOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		return
	}

	from := cb.state
	cb.state = StateHalfOpen
	cb.lastStateTime = time.Now()
	atomic.StoreInt64(&cb.halfOpenCount, 0)

	cb.metrics.State = StateHalfOpen
	cb.metrics.LastStateChange = time.Now()

	zzlog.Infow("circuit breaker half-open",
		zap.String("name", cb.config.Name),
		zap.String("from", from.String()))

	if cb.onStateChange != nil {
		cb.onStateChange(from, StateHalfOpen)
	}
}

// calculateErrorRate calculates the current error rate
func (cb *breaker) calculateErrorRate() float64 {
	rwMetrics := cb.rollingWindow.getMetrics()

	var totalRequests, failedRequests int64
	for _, bucket := range rwMetrics.Buckets {
		totalRequests += bucket.TotalRequests
		failedRequests += bucket.FailedRequests
	}

	if totalRequests == 0 {
		return 0.0
	}

	return float64(failedRequests) / float64(totalRequests)
}

// stateManager manages circuit breaker state transitions
func (cb *breaker) stateManager() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cb.stopChan:
			return
		case <-ticker.C:
			cb.checkState()
		}
	}
}

// checkState checks if state should be transitioned
func (cb *breaker) checkState() {
	cb.mu.RLock()
	state := cb.state
	lastStateTime := cb.lastStateTime
	cb.mu.RUnlock()

	switch state {
	case StateOpen:
		// Check if sleep window has passed
		if time.Since(lastStateTime) >= cb.config.SleepWindow {
			cb.transitionToHalfOpen()
		}

	case StateHalfOpen:
		// Reset half-open count periodically
		if time.Since(lastStateTime) > cb.config.SleepWindow {
			atomic.StoreInt64(&cb.halfOpenCount, 0)
		}
	}
}

// record records a request result in the rolling window
func (rw *rollingWindow) record(success bool) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	// Rotate buckets if needed
	now := time.Now()
	if now.Sub(rw.lastRotate) >= rw.bucketSize {
		rw.rotate(now)
	}

	// Update current bucket
	bucket := &rw.buckets[rw.currentIndex]
	bucket.TotalRequests++
	if success {
		bucket.SuccessfulRequests++
	} else {
		bucket.FailedRequests++
	}

	// Calculate bucket error rate
	if bucket.TotalRequests > 0 {
		bucket.ErrorRate = float64(bucket.FailedRequests) / float64(bucket.TotalRequests)
	}
}

// rotate rotates the rolling window buckets
func (rw *rollingWindow) rotate(now time.Time) {
	rw.currentIndex = (rw.currentIndex + 1) % rw.bucketCount
	rw.buckets[rw.currentIndex] = BucketMetrics{
		StartTime: now,
	}
	rw.lastRotate = now
}

// getMetrics returns the current rolling window metrics
func (rw *rollingWindow) getMetrics() *RollingWindowMetrics {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	// Rotate buckets if needed
	now := time.Now()
	if now.Sub(rw.lastRotate) >= rw.bucketSize {
		rw.mu.RUnlock()
		rw.mu.Lock()
		rw.rotate(now)
		rw.mu.Unlock()
		rw.mu.RLock()
	}

	// Copy buckets
	buckets := make([]BucketMetrics, rw.bucketCount)
	copy(buckets, rw.buckets)

	return &RollingWindowMetrics{
		WindowSize:         rw.windowSize,
		BucketCount:        rw.bucketCount,
		Buckets:            buckets,
		CurrentBucketIndex: rw.currentIndex,
	}
}

// reset resets the rolling window
func (rw *rollingWindow) reset() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	for i := range rw.buckets {
		rw.buckets[i] = BucketMetrics{
			StartTime: time.Now().Add(-time.Duration(i) * rw.bucketSize),
		}
	}
	rw.currentIndex = 0
	rw.lastRotate = time.Now()
}
