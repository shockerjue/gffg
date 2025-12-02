package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shockerjue/gffg/circuitbreaker"
	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

// Manager manages both rate limiting and circuit breaking for services
type Manager struct {
	mu sync.RWMutex

	// Rate limiters
	limiters       map[string]Limiter
	limiterConfigs map[string]Limit

	// Circuit breakers
	circuitBreakers map[string]circuitbreaker.CircuitBreaker
	cbConfigs       map[string]circuitbreaker.Config

	// Configuration
	config *ManagerConfig

	// Metrics
	metrics *ManagerMetrics

	// Cleanup
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// ManagerConfig holds manager configuration
type ManagerConfig struct {
	// Default rate limit configuration
	DefaultRateLimit Limit

	// Default circuit breaker configuration
	DefaultCircuitBreaker circuitbreaker.Config

	// Storage backend for rate limiters
	StorageBackend StorageBackend

	// Redis configuration (if using Redis storage)
	RedisConfig *RedisConfig

	// Local configuration (if using local storage)
	LocalConfig *LocalConfig

	// Cleanup interval for expired entries
	CleanupInterval time.Duration

	// Enable metrics collection
	MetricsEnabled bool

	// Enable dynamic adjustment of limits
	DynamicLimits bool

	// Maximum number of limiters to keep in memory
	MaxLimiters int

	// Maximum number of circuit breakers to keep in memory
	MaxCircuitBreakers int
}

// ManagerMetrics holds manager metrics
type ManagerMetrics struct {
	// Rate limiter metrics
	RateLimitAllowed       int64
	RateLimitDenied        int64
	RateLimitWaitTimeTotal int64

	// Circuit breaker metrics
	CircuitBreakerRequests int64
	CircuitBreakerSuccess  int64
	CircuitBreakerFailures int64
	CircuitBreakerRejected int64

	// Manager metrics
	LimitersCreated        int64
	LimitersRemoved        int64
	CircuitBreakersCreated int64
	CircuitBreakersRemoved int64
}

// NewManager creates a new rate limit and circuit breaker manager
func NewManager(config *ManagerConfig) (*Manager, error) {
	if config == nil {
		config = &ManagerConfig{
			DefaultRateLimit: Limit{
				Rate:      100,
				Burst:     200,
				Period:    time.Second,
				Algorithm: TokenBucket,
			},
			DefaultCircuitBreaker: circuitbreaker.DefaultConfig("default"),
			StorageBackend:        LocalStorage,
			LocalConfig: &LocalConfig{
				MaxKeys:         10000,
				CleanupInterval: 5 * time.Minute,
			},
			CleanupInterval:    5 * time.Minute,
			MetricsEnabled:     true,
			DynamicLimits:      false,
			MaxLimiters:        1000,
			MaxCircuitBreakers: 1000,
		}
	}

	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	manager := &Manager{
		limiters:        make(map[string]Limiter),
		limiterConfigs:  make(map[string]Limit),
		circuitBreakers: make(map[string]circuitbreaker.CircuitBreaker),
		cbConfigs:       make(map[string]circuitbreaker.Config),
		config:          config,
		metrics:         &ManagerMetrics{},
		stopChan:        make(chan struct{}),
	}

	// Start cleanup goroutine
	manager.cleanupTicker = time.NewTicker(config.CleanupInterval)
	go manager.cleanup()

	return manager, nil
}

// Allow checks if a request is allowed considering both rate limiting and circuit breaking
func (m *Manager) Allow(ctx context.Context, service, method string) (bool, error) {
	key := fmt.Sprintf("%s:%s", service, method)

	// Check circuit breaker first
	if cb, exists := m.getCircuitBreaker(key); exists {
		if !cb.AllowRequest(ctx) {
			if m.config.MetricsEnabled {
				m.metrics.CircuitBreakerRejected++
			}
			return false, circuitbreaker.ErrCircuitOpen
		}
	}

	// Check rate limiter
	if limiter, exists := m.getLimiter(key); exists {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			if rle, ok := err.(*RateLimitError); ok && rle.Code == "LIMIT_EXCEEDED" {
				if m.config.MetricsEnabled {
					m.metrics.RateLimitDenied++
				}
				return false, err
			}
			return false, err
		}

		if !allowed {
			if m.config.MetricsEnabled {
				m.metrics.RateLimitDenied++
			}
			return false, ErrLimitExceeded
		}

		if m.config.MetricsEnabled {
			m.metrics.RateLimitAllowed++
		}
	}

	if m.config.MetricsEnabled {
		m.metrics.CircuitBreakerRequests++
	}

	return true, nil
}

// Execute executes an operation with rate limiting and circuit breaking protection
func (m *Manager) Execute(ctx context.Context, service, method string, operation func() (interface{}, error)) (interface{}, error) {
	key := fmt.Sprintf("%s:%s", service, method)

	// Get or create circuit breaker
	cb, err := m.getOrCreateCircuitBreaker(key)
	if err != nil {
		return nil, err
	}

	// Execute with circuit breaker protection
	result, err := cb.Execute(ctx, operation)

	// Record result for rate limiting adjustment
	if err != nil {
		if m.config.MetricsEnabled {
			m.metrics.CircuitBreakerFailures++
		}
	} else {
		if m.config.MetricsEnabled {
			m.metrics.CircuitBreakerSuccess++
		}
	}

	// Adjust rate limits dynamically if enabled
	if m.config.DynamicLimits {
		m.adjustLimits(key, err == nil)
	}

	return result, err
}

// SetRateLimit sets or updates the rate limit for a service method
func (m *Manager) SetRateLimit(ctx context.Context, service, method string, limit Limit) error {
	key := fmt.Sprintf("%s:%s", service, method)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the configuration
	m.limiterConfigs[key] = limit

	// Create or update the limiter
	if limiter, exists := m.limiters[key]; exists {
		if err := limiter.UpdateLimit(ctx, key, limit); err != nil {
			return err
		}
	} else {
		// Create new limiter
		limiter, err := NewTokenBucket(&Config{
			DefaultLimit:    limit,
			StorageBackend:  m.config.StorageBackend,
			RedisConfig:     m.config.RedisConfig,
			LocalConfig:     m.config.LocalConfig,
			CleanupInterval: m.config.CleanupInterval,
			MetricsEnabled:  m.config.MetricsEnabled,
			DynamicLimits:   m.config.DynamicLimits,
		})
		if err != nil {
			return err
		}

		m.limiters[key] = limiter
		if m.config.MetricsEnabled {
			m.metrics.LimitersCreated++
		}
	}

	return nil
}

// SetCircuitBreaker sets or updates the circuit breaker for a service method
func (m *Manager) SetCircuitBreaker(service, method string, config circuitbreaker.Config) error {
	key := fmt.Sprintf("%s:%s", service, method)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the configuration
	m.cbConfigs[key] = config

	// Create or update the circuit breaker
	if cb, exists := m.circuitBreakers[key]; exists {
		// For existing circuit breakers, we need to create a new one with updated config
		cb.Close()
	}

	cb, err := circuitbreaker.NewCircuitBreaker(config)
	if err != nil {
		return err
	}

	m.circuitBreakers[key] = cb
	if m.config.MetricsEnabled {
		m.metrics.CircuitBreakersCreated++
	}

	return nil
}

// GetRateLimit returns the current rate limit for a service method
func (m *Manager) GetRateLimit(ctx context.Context, service, method string) (Limit, error) {
	key := fmt.Sprintf("%s:%s", service, method)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit, exists := m.limiterConfigs[key]; exists {
		return limit, nil
	}

	return m.config.DefaultRateLimit, nil
}

// GetCircuitBreakerConfig returns the circuit breaker configuration for a service method
func (m *Manager) GetCircuitBreakerConfig(service, method string) (circuitbreaker.Config, error) {
	key := fmt.Sprintf("%s:%s", service, method)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if config, exists := m.cbConfigs[key]; exists {
		return config, nil
	}

	config := m.config.DefaultCircuitBreaker
	config.Name = key
	return config, nil
}

// GetMetrics returns the current metrics
func (m *Manager) GetMetrics() *ManagerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &ManagerMetrics{
		RateLimitAllowed:       m.metrics.RateLimitAllowed,
		RateLimitDenied:        m.metrics.RateLimitDenied,
		RateLimitWaitTimeTotal: m.metrics.RateLimitWaitTimeTotal,
		CircuitBreakerRequests: m.metrics.CircuitBreakerRequests,
		CircuitBreakerSuccess:  m.metrics.CircuitBreakerSuccess,
		CircuitBreakerFailures: m.metrics.CircuitBreakerFailures,
		CircuitBreakerRejected: m.metrics.CircuitBreakerRejected,
		LimitersCreated:        m.metrics.LimitersCreated,
		LimitersRemoved:        m.metrics.LimitersRemoved,
		CircuitBreakersCreated: m.metrics.CircuitBreakersCreated,
		CircuitBreakersRemoved: m.metrics.CircuitBreakersRemoved,
	}
}

// Close releases all resources
func (m *Manager) Close() error {
	close(m.stopChan)
	m.cleanupTicker.Stop()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all limiters
	for key, limiter := range m.limiters {
		limiter.Close()
		delete(m.limiters, key)
	}

	// Close all circuit breakers
	for key, cb := range m.circuitBreakers {
		cb.Close()
		delete(m.circuitBreakers, key)
	}

	return nil
}

// getLimiter gets a limiter for the given key
func (m *Manager) getLimiter(key string) (Limiter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limiter, exists := m.limiters[key]
	return limiter, exists
}

// getCircuitBreaker gets a circuit breaker for the given key
func (m *Manager) getCircuitBreaker(key string) (circuitbreaker.CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cb, exists := m.circuitBreakers[key]
	return cb, exists
}

// getOrCreateCircuitBreaker gets or creates a circuit breaker for the given key
func (m *Manager) getOrCreateCircuitBreaker(key string) (circuitbreaker.CircuitBreaker, error) {
	m.mu.RLock()
	cb, exists := m.circuitBreakers[key]
	m.mu.RUnlock()

	if exists {
		return cb, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring lock
	if cb, exists = m.circuitBreakers[key]; exists {
		return cb, nil
	}

	// Get configuration
	config, exists := m.cbConfigs[key]
	if !exists {
		config = m.config.DefaultCircuitBreaker
		config.Name = key
		m.cbConfigs[key] = config
	}

	// Create new circuit breaker
	cb, err := circuitbreaker.NewCircuitBreaker(config)
	if err != nil {
		return nil, err
	}

	m.circuitBreakers[key] = cb
	if m.config.MetricsEnabled {
		m.metrics.CircuitBreakersCreated++
	}

	return cb, nil
}

// adjustLimits adjusts rate limits based on operation success/failure
func (m *Manager) adjustLimits(key string, success bool) {
	m.mu.RLock()
	limiter, exists := m.limiters[key]
	config, configExists := m.limiterConfigs[key]
	m.mu.RUnlock()

	if !exists || !configExists {
		return
	}

	// Simple adaptive algorithm: increase limit on success, decrease on failure
	newLimit := config
	if success {
		// Increase rate by 10% (up to 10x original)
		newLimit.Rate = min(config.Rate*1.1, config.Rate*10)
	} else {
		// Decrease rate by 20% (down to 10% of original)
		newLimit.Rate = max(config.Rate*0.8, config.Rate*0.1)
	}

	// Update burst size proportionally
	newLimit.Burst = int(float64(config.Burst) * (newLimit.Rate / config.Rate))

	// Update the limiter
	if err := limiter.UpdateLimit(context.Background(), key, newLimit); err != nil {
		zzlog.Warnw("failed to adjust rate limit",
			zap.String("key", key),
			zap.Error(err))
	} else {
		m.mu.Lock()
		m.limiterConfigs[key] = newLimit
		m.mu.Unlock()
	}
}

// cleanup removes old limiters and circuit breakers
func (m *Manager) cleanup() {
	for {
		select {
		case <-m.stopChan:
			return
		case <-m.cleanupTicker.C:
			m.cleanupExpired()
		}
	}
}

// cleanupExpired removes expired entries
func (m *Manager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up limiters if we have too many
	if len(m.limiters) > m.config.MaxLimiters {
		m.cleanupOldEntries(m.limiters, m.limiterConfigs, m.config.MaxLimiters)
		if m.config.MetricsEnabled {
			m.metrics.LimitersRemoved += int64(len(m.limiters) - m.config.MaxLimiters)
		}
	}

	// Clean up circuit breakers if we have too many
	if len(m.circuitBreakers) > m.config.MaxCircuitBreakers {
		m.cleanupOldEntriesCB(m.circuitBreakers, m.cbConfigs, m.config.MaxCircuitBreakers)
		if m.config.MetricsEnabled {
			m.metrics.CircuitBreakersRemoved += int64(len(m.circuitBreakers) - m.config.MaxCircuitBreakers)
		}
	}

	if zzlog.IsDebugEnabled() {
		zzlog.Debugw("rate limit manager cleanup",
			zap.Int("limiters", len(m.limiters)),
			zap.Int("circuit_breakers", len(m.circuitBreakers)),
			zap.Int("max_limiters", m.config.MaxLimiters),
			zap.Int("max_circuit_breakers", m.config.MaxCircuitBreakers))
	}
}

// cleanupOldEntries removes the oldest entries from a map
func (m *Manager) cleanupOldEntries(limiters map[string]Limiter, configs map[string]Limit, maxSize int) {
	// For simplicity, we'll just remove random entries
	// In a real implementation, you might want to track last access time
	toRemove := len(limiters) - maxSize
	if toRemove <= 0 {
		return
	}

	removed := 0
	for key := range limiters {
		if removed >= toRemove {
			break
		}

		if limiter, exists := limiters[key]; exists {
			limiter.Close()
			delete(limiters, key)
			delete(configs, key)
			removed++
		}
	}
}

// cleanupOldEntriesCB removes the oldest entries from circuit breaker maps
func (m *Manager) cleanupOldEntriesCB(cbs map[string]circuitbreaker.CircuitBreaker, configs map[string]circuitbreaker.Config, maxSize int) {
	toRemove := len(cbs) - maxSize
	if toRemove <= 0 {
		return
	}

	removed := 0
	for key := range cbs {
		if removed >= toRemove {
			break
		}

		if cb, exists := cbs[key]; exists {
			cb.Close()
			delete(cbs, key)
			delete(configs, key)
			removed++
		}
	}
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
