package common

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus int

const (
	// HealthStatusUnknown indicates the health status is unknown
	HealthStatusUnknown HealthStatus = iota
	// HealthStatusHealthy indicates the component is healthy
	HealthStatusHealthy
	// HealthStatusDegraded indicates the component is degraded but still functional
	HealthStatusDegraded
	// HealthStatusUnhealthy indicates the component is unhealthy
	HealthStatusUnhealthy
)

// String returns the string representation of the health status
func (h HealthStatus) String() string {
	switch h {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	case HealthStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheck represents a health check for a component
type HealthCheck struct {
	Name        string
	Description string
	Check       func(ctx context.Context) error
	Timeout     time.Duration
	Interval    time.Duration
	Critical    bool // If true, failure makes the component unhealthy
}

// HealthChecker manages health checks for components
type HealthChecker struct {
	mu          sync.RWMutex
	checks      map[string]*HealthCheck
	statuses    map[string]HealthStatus
	lastChecked map[string]time.Time
	errors      map[string]error
	stopChan    chan struct{}
	running     bool
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks:      make(map[string]*HealthCheck),
		statuses:    make(map[string]HealthStatus),
		lastChecked: make(map[string]time.Time),
		errors:      make(map[string]error),
		stopChan:    make(chan struct{}),
	}
}

// Register adds a health check to the checker
func (h *HealthChecker) Register(check *HealthCheck) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if check.Name == "" {
		return fmt.Errorf("health check name cannot be empty")
	}

	if check.Check == nil {
		return fmt.Errorf("health check function cannot be nil")
	}

	if check.Timeout == 0 {
		check.Timeout = 5 * time.Second
	}

	if check.Interval == 0 {
		check.Interval = 30 * time.Second
	}

	h.checks[check.Name] = check
	h.statuses[check.Name] = HealthStatusUnknown
	h.errors[check.Name] = nil

	return nil
}

// Deregister removes a health check from the checker
func (h *HealthChecker) Deregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.checks, name)
	delete(h.statuses, name)
	delete(h.lastChecked, name)
	delete(h.errors, name)
}

// Run starts the health checker
func (h *HealthChecker) Run() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	go h.run()
}

// Stop stops the health checker
func (h *HealthChecker) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}

	close(h.stopChan)
	h.running = false
}

// run executes health checks at their specified intervals
func (h *HealthChecker) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.executeChecks()
		}
	}
}

// executeChecks runs all health checks that are due
func (h *HealthChecker) executeChecks() {
	h.mu.RLock()
	checks := make([]*HealthCheck, 0, len(h.checks))
	now := time.Now()

	for _, check := range h.checks {
		lastChecked, exists := h.lastChecked[check.Name]
		if !exists || now.Sub(lastChecked) >= check.Interval {
			checks = append(checks, check)
		}
	}
	h.mu.RUnlock()

	for _, check := range checks {
		h.executeCheck(check)
	}
}

// executeCheck runs a single health check
func (h *HealthChecker) executeCheck(check *HealthCheck) {
	ctx, cancel := context.WithTimeout(context.Background(), check.Timeout)
	defer cancel()

	ch := make(chan error, 1)
	go func() {
		ch <- check.Check(ctx)
	}()

	var err error
	select {
	case err = <-ch:
		// Check completed
	case <-ctx.Done():
		err = fmt.Errorf("health check timeout after %v", check.Timeout)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastChecked[check.Name] = time.Now()
	h.errors[check.Name] = err

	if err == nil {
		h.statuses[check.Name] = HealthStatusHealthy
	} else if check.Critical {
		h.statuses[check.Name] = HealthStatusUnhealthy
	} else {
		h.statuses[check.Name] = HealthStatusDegraded
	}
}

// GetStatus returns the health status of a specific check
func (h *HealthChecker) GetStatus(name string) (HealthStatus, error, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status, exists := h.statuses[name]
	if !exists {
		return HealthStatusUnknown, fmt.Errorf("health check not found: %s", name), time.Time{}
	}

	err := h.errors[name]
	lastChecked := h.lastChecked[name]

	return status, err, lastChecked
}

// GetOverallStatus returns the overall health status
func (h *HealthChecker) GetOverallStatus() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.checks) == 0 {
		return HealthStatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false

	for name, check := range h.checks {
		status := h.statuses[name]
		switch status {
		case HealthStatusUnhealthy:
			if check.Critical {
				return HealthStatusUnhealthy
			}
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		case HealthStatusUnknown:
			// Treat unknown as degraded for overall status
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// GetStatusReport returns a detailed status report
func (h *HealthChecker) GetStatusReport() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	report := make(map[string]interface{})
	report["overall_status"] = h.GetOverallStatus().String()
	report["timestamp"] = time.Now().UTC().Format(time.RFC3339)

	checks := make([]map[string]interface{}, 0, len(h.checks))
	for name, check := range h.checks {
		status := h.statuses[name]
		err := h.errors[name]
		lastChecked := h.lastChecked[name]

		checkInfo := map[string]interface{}{
			"name":         check.Name,
			"description":  check.Description,
			"status":       status.String(),
			"critical":     check.Critical,
			"last_checked": lastChecked.Format(time.RFC3339),
			"interval":     check.Interval.String(),
			"timeout":      check.Timeout.String(),
		}

		if err != nil {
			checkInfo["error"] = err.Error()
		}

		checks = append(checks, checkInfo)
	}

	report["checks"] = checks
	return report
}

// Configuration validation utilities

// ValidatePort validates that a port number is valid
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of valid range (1-65535)", port)
	}
	return nil
}

// ValidateTimeout validates that a timeout duration is valid
func ValidateTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if timeout > 10*time.Minute {
		return fmt.Errorf("timeout %v is too long (max 10 minutes)", timeout)
	}
	return nil
}

// ValidateRetryConfig validates retry configuration
func ValidateRetryConfig(maxRetries int, backoff time.Duration) error {
	if maxRetries < 0 {
		return fmt.Errorf("maxRetries cannot be negative")
	}
	if maxRetries > 100 {
		return fmt.Errorf("maxRetries %d is too high (max 100)", maxRetries)
	}
	if backoff < 0 {
		return fmt.Errorf("backoff cannot be negative")
	}
	if backoff > 10*time.Second {
		return fmt.Errorf("backoff %v is too long (max 10 seconds)", backoff)
	}
	return nil
}

// ValidatePoolSize validates connection pool size
func ValidatePoolSize(poolSize int) error {
	if poolSize <= 0 {
		return fmt.Errorf("poolSize must be positive")
	}
	if poolSize > 1000 {
		return fmt.Errorf("poolSize %d is too large (max 1000)", poolSize)
	}
	return nil
}

// ValidateBufferSize validates buffer size configuration
func ValidateBufferSize(bufferSize int) error {
	if bufferSize <= 0 {
		return fmt.Errorf("bufferSize must be positive")
	}
	if bufferSize > 100*1024*1024 { // 100MB
		return fmt.Errorf("bufferSize %d bytes is too large (max 100MB)", bufferSize)
	}
	return nil
}

// ValidateRateLimit validates rate limit configuration
func ValidateRateLimit(rateLimit int, burst int) error {
	if rateLimit <= 0 {
		return fmt.Errorf("rateLimit must be positive")
	}
	if burst <= 0 {
		return fmt.Errorf("burst must be positive")
	}
	if burst > 10*rateLimit {
		return fmt.Errorf("burst %d is too high relative to rateLimit %d", burst, rateLimit)
	}
	return nil
}

// ConfigValidator validates configuration values
type ConfigValidator struct {
	errors []string
}

// NewConfigValidator creates a new ConfigValidator
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		errors: make([]string, 0),
	}
}

// Validate adds a validation check
func (v *ConfigValidator) Validate(name string, value interface{}, validator func(interface{}) error) {
	if err := validator(value); err != nil {
		v.errors = append(v.errors, fmt.Sprintf("%s: %v", name, err))
	}
}

// ValidatePort validates a port configuration value
func (v *ConfigValidator) ValidatePort(name string, port int) {
	v.Validate(name, port, func(val interface{}) error {
		return ValidatePort(val.(int))
	})
}

// ValidateTimeout validates a timeout configuration value
func (v *ConfigValidator) ValidateTimeout(name string, timeout time.Duration) {
	v.Validate(name, timeout, func(val interface{}) error {
		return ValidateTimeout(val.(time.Duration))
	})
}

// ValidatePoolSize validates a pool size configuration value
func (v *ConfigValidator) ValidatePoolSize(name string, poolSize int) {
	v.Validate(name, poolSize, func(val interface{}) error {
		return ValidatePoolSize(val.(int))
	})
}

// ValidateBufferSize validates a buffer size configuration value
func (v *ConfigValidator) ValidateBufferSize(name string, bufferSize int) {
	v.Validate(name, bufferSize, func(val interface{}) error {
		return ValidateBufferSize(val.(int))
	})
}

// ValidateRateLimit validates a rate limit configuration value
func (v *ConfigValidator) ValidateRateLimit(name string, rateLimit, burst int) {
	v.Validate(name, map[string]int{"rateLimit": rateLimit, "burst": burst}, func(val interface{}) error {
		m := val.(map[string]int)
		return ValidateRateLimit(m["rateLimit"], m["burst"])
	})
}

// HasErrors returns true if there are validation errors
func (v *ConfigValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// GetErrors returns all validation errors
func (v *ConfigValidator) GetErrors() []string {
	return v.errors
}

// GetErrorString returns validation errors as a single string
func (v *ConfigValidator) GetErrorString() string {
	if len(v.errors) == 0 {
		return ""
	}
	errorStr := "Configuration validation errors:\n"
	for i, err := range v.errors {
		errorStr += fmt.Sprintf("  %d. %s\n", i+1, err)
	}
	return errorStr
}
