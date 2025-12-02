# GFFG Rate Limiting and Circuit Breaking

## Overview

GFFG framework now includes comprehensive rate limiting and circuit breaking capabilities to enhance service reliability, prevent cascading failures, and protect backend services from overload. These features can be used both on the client side (outgoing requests) and server side (incoming requests).

## Features

### Rate Limiting
- **Token Bucket Algorithm**: Default implementation with configurable rate and burst
- **Multiple Storage Backends**: Local memory, Redis, and cluster storage
- **Dynamic Rate Adjustment**: Adaptive limits based on system load and error rates
- **Per-Service/Method Limits**: Granular control over different API endpoints
- **Metrics Collection**: Comprehensive monitoring of allowed/denied requests

### Circuit Breaking
- **Three-State Pattern**: Closed, Open, and Half-Open states
- **Configurable Thresholds**: Error rate, request count, and sleep windows
- **Rolling Window Metrics**: Time-based metrics collection
- **State Change Callbacks**: Notifications for state transitions
- **Health Monitoring**: Automatic recovery and testing

### Combined Management
- **Unified Manager**: Single interface for both rate limiting and circuit breaking
- **Method-Level Configuration**: Different settings per API method
- **Dynamic Adjustment**: Automatic tuning based on performance
- **Resource Management**: Automatic cleanup of unused limiters and breakers

## Installation

The rate limiting and circuit breaking features are built into the GFFG framework. No additional installation is required.

## Quick Start

### Basic Rate Limiting

```go
import "github.com/shockerjue/gffg/ratelimit"

// Create a token bucket rate limiter
config := &ratelimit.Config{
    DefaultLimit: ratelimit.Limit{
        Rate:      100,  // 100 requests per second
        Burst:     200,  // Allow bursts up to 200 requests
        Period:    time.Second,
        Algorithm: ratelimit.TokenBucket,
    },
    StorageBackend: ratelimit.LocalStorage,
    MetricsEnabled: true,
}

limiter, err := ratelimit.NewTokenBucket(config)
if err != nil {
    log.Fatal(err)
}
defer limiter.Close()

// Check if a request is allowed
allowed, err := limiter.Allow(ctx, "service:method")
if err != nil {
    // Handle error
}
if !allowed {
    // Rate limit exceeded
}
```

### Basic Circuit Breaking

```go
import "github.com/shockerjue/gffg/circuitbreaker"

// Create a circuit breaker
config := circuitbreaker.Config{
    Name:                "payment-service",
    ErrorThreshold:      0.5,  // 50% error rate
    RequestThreshold:    20,   // Minimum 20 requests
    SleepWindow:         5 * time.Second,
    HalfOpenMaxRequests: 5,
    Timeout:             10 * time.Second,
    RollingWindow:       30 * time.Second,
    BucketCount:         10,
    MetricsEnabled:      true,
}

cb, err := circuitbreaker.NewCircuitBreaker(config)
if err != nil {
    log.Fatal(err)
}
defer cb.Close()

// Execute with circuit breaker protection
result, err := cb.Execute(ctx, func() (interface{}, error) {
    // Your operation here
    return callExternalService()
})
```

### Combined Manager

```go
import "github.com/shockerjue/gffg/ratelimit"

// Create a manager for both rate limiting and circuit breaking
mgrConfig := &ratelimit.ManagerConfig{
    DefaultRateLimit: ratelimit.Limit{
        Rate:      100,
        Burst:     200,
        Period:    time.Second,
        Algorithm: ratelimit.TokenBucket,
    },
    DefaultCircuitBreaker: circuitbreaker.DefaultConfig("default"),
    StorageBackend:        ratelimit.LocalStorage,
    MetricsEnabled:        true,
    DynamicLimits:         true,
}

mgr, err := ratelimit.NewManager(mgrConfig)
if err != nil {
    log.Fatal(err)
}
defer mgr.Close()

// Execute with both protections
result, err := mgr.Execute(ctx, "order-service", "createOrder", func() (interface{}, error) {
    return processOrder()
})
```

## Configuration

### Rate Limiter Configuration

```go
type Config struct {
    DefaultLimit    Limit           // Default rate limit
    StorageBackend  StorageBackend  // Local, Redis, or Cluster
    RedisConfig     *RedisConfig    // Redis-specific config
    LocalConfig     *LocalConfig    // Local storage config
    CleanupInterval time.Duration   // Cleanup interval
    MetricsEnabled  bool            // Enable metrics
    DynamicLimits   bool            // Enable dynamic adjustment
}

type Limit struct {
    Rate      float64       // Requests per period
    Burst     int           // Maximum burst size
    Period    time.Duration // Time period (e.g., time.Second)
    Algorithm Algorithm     // TokenBucket, LeakyBucket, etc.
}
```

### Circuit Breaker Configuration

```go
type Config struct {
    Name                string        // Circuit breaker name
    ErrorThreshold      float64       // Error percentage to open (0.0-1.0)
    RequestThreshold    int64         // Minimum requests before opening
    SleepWindow         time.Duration // How long to stay open
    HalfOpenMaxRequests int64         // Max requests in half-open state
    Timeout             time.Duration // Operation timeout
    RollingWindow       time.Duration // Metrics rolling window
    BucketCount         int           // Buckets in rolling window
    FailurePredicate    func(error) bool // What counts as failure
    OnStateChange       func(from, to State) // State change callback
    MetricsEnabled      bool          // Enable metrics
}
```

## Integration with GFFG Framework

### Client-Side Integration

```go
import "github.com/shockerjue/gffg/client"

// Create client with rate limiting and circuit breaking
client := client.NewClient("service-group",
    client.RateLimitConfig(ratelimit.Limit{
        Rate:   1000,
        Burst:  2000,
        Period: time.Second,
    }),
    client.CircuitBreakerConfig(circuitbreaker.Config{
        Name:           "client-circuit-breaker",
        ErrorThreshold: 0.3,
        SleepWindow:    5 * time.Second,
    }),
)

// The client will automatically apply rate limiting and circuit breaking
```

### Server-Side Integration

```go
import "github.com/shockerjue/gffg/server"

// Create server with rate limiting and circuit breaking
server := server.NewServer("config.xml",
    server.RateLimitConfig(ratelimit.Limit{
        Rate:   500,
        Burst:  1000,
        Period: time.Second,
    }),
    server.CircuitBreakerConfig(circuitbreaker.Config{
        Name:           "server-circuit-breaker",
        ErrorThreshold: 0.4,
        SleepWindow:    3 * time.Second,
    }),
)

// The server will automatically apply protections to incoming requests
```

## Configuration File Example

Add these sections to your XML configuration file:

```xml
<server>
    <!-- Rate limiting configuration -->
    <rate_limit>
        <enabled>true</enabled>
        <global>1000</global>        <!-- Global rate limit -->
        <per_method>100</per_method> <!-- Per-method rate limit -->
        <burst>2000</burst>          <!-- Burst size -->
        <dynamic>true</dynamic>      <!-- Enable dynamic adjustment -->
        <max_keys>10000</max_keys>   <!-- Maximum keys in storage -->
    </rate_limit>
    
    <!-- Circuit breaker configuration -->
    <circuit_breaker>
        <enabled>true</enabled>
        <error_threshold>0.5</error_threshold>
        <request_threshold>20</request_threshold>
        <sleep_window>5</sleep_window> <!-- seconds -->
        <half_open_max_requests>5</half_open_max_requests>
        <timeout>30</timeout>          <!-- seconds -->
        <rolling_window>10</rolling_window> <!-- seconds -->
        <bucket_count>10</bucket_count>
        <max_circuit_breakers>1000</max_circuit_breakers>
    </circuit_breaker>
</server>

<client>
    <!-- Client-side rate limiting -->
    <rate_limit>
        <enabled>true</enabled>
        <default_rate>500</default_rate>
        <default_burst>1000</default_burst>
    </rate_limit>
    
    <!-- Client-side circuit breaking -->
    <circuit_breaker>
        <enabled>true</enabled>
        <error_threshold>0.4</error_threshold>
        <sleep_window>3</sleep_window>
    </circuit_breaker>
</client>
```

## Advanced Usage

### Dynamic Rate Adjustment

```go
// Enable dynamic limits in configuration
config := &ratelimit.Config{
    DynamicLimits: true,
    // ... other config
}

// The system will automatically adjust rates based on:
// - Success/failure rates
// - System load
// - Response times
```

### Custom Failure Predicate

```go
// Define what errors should trigger circuit breaking
config := circuitbreaker.Config{
    FailurePredicate: func(err error) bool {
        // Only consider network errors and 5xx errors as failures
        if err == nil {
            return false
        }
        
        // Check error type
        if strings.Contains(err.Error(), "timeout") ||
           strings.Contains(err.Error(), "network") ||
           strings.Contains(err.Error(), "5xx") {
            return true
        }
        
        return false
    },
    // ... other config
}
```

### State Change Notifications

```go
config := circuitbreaker.Config{
    OnStateChange: func(from, to circuitbreaker.State) {
        // Log state changes
        log.Printf("Circuit breaker %s changed from %s to %s", 
            config.Name, from.String(), to.String())
        
        // Send metrics
        metrics.Increment("circuit_breaker.state_change", 1, 
            "from:" + from.String(),
            "to:" + to.String())
        
        // Trigger alerts for critical state changes
        if to == circuitbreaker.StateOpen {
            sendAlert("Circuit breaker opened: " + config.Name)
        }
    },
    // ... other config
}
```

### Redis Storage Backend

```go
config := &ratelimit.Config{
    StorageBackend: ratelimit.RedisStorage,
    RedisConfig: &ratelimit.RedisConfig{
        Addrs:    []string{"localhost:6379"},
        Password: "", // Optional
        DB:       0,
        PoolSize: 10,
    },
    // ... other config
}
```

## Monitoring and Metrics

### Available Metrics

#### Rate Limiter Metrics
- `ratelimit_allowed_total`: Total allowed requests
- `ratelimit_denied_total`: Total denied requests
- `ratelimit_wait_time_total`: Total wait time in nanoseconds
- `ratelimit_limit_updates_total`: Number of limit updates
- `ratelimit_storage_errors_total`: Storage errors

#### Circuit Breaker Metrics
- `circuitbreaker_requests_total`: Total requests
- `circuitbreaker_success_total`: Successful requests
- `circuitbreaker_failures_total`: Failed requests
- `circuitbreaker_rejected_total`: Rejected requests (circuit open)
- `circuitbreaker_error_rate`: Current error rate (0.0-1.0)
- `circuitbreaker_state`: Current state (0=closed, 1=open, 2=half-open)

### Integration with Prometheus

```go
import "github.com/prometheus/client_golang/prometheus"

// Register metrics collectors
prometheus.MustRegister(ratelimitMetricsCollector)
prometheus.MustRegister(circuitbreakerMetricsCollector)
```

## Best Practices

### 1. Start Conservative
Begin with conservative limits and gradually adjust based on monitoring:
- Start with lower rate limits
- Use higher error thresholds initially
- Monitor system behavior

### 2. Monitor Closely
- Set up alerts for circuit breaker state changes
- Monitor rate limit denial rates
- Track error rates and response times

### 3. Use Different Configurations
- Use stricter limits for write operations
- Use more lenient limits for read operations
- Adjust based on service criticality

### 4. Test Failure Scenarios
- Test circuit breaker behavior under failure conditions
- Verify rate limiting under load
- Ensure proper recovery mechanisms

### 5. Consider Service Dependencies
- Coordinate rate limits across dependent services
- Use circuit breaking to prevent cascading failures
- Implement retry strategies with backoff

## Troubleshooting

### Common Issues

#### 1. Too Many Rate Limit Denials
- **Cause**: Rate limits set too low
- **Solution**: Increase rate limits or burst size
- **Alternative**: Implement request queuing or caching

#### 2. Circuit Breaker Stays Open
- **Cause**: Underlying service not recovering
- **Solution**: Increase sleep window or adjust error threshold
- **Alternative**: Implement fallback mechanisms

#### 3. High Memory Usage
- **Cause**: Too many rate limit keys stored
- **Solution**: Increase cleanup frequency or reduce max keys
- **Alternative**: Use Redis storage backend

#### 4. Inconsistent Rate Limiting
- **Cause**: Multiple instances without shared storage
- **Solution**: Use Redis or cluster storage backend
- **Alternative**: Implement consistent hashing

### Debugging Commands

```go
// Check circuit breaker state
state := cb.GetState()
fmt.Printf("Circuit breaker state: %s\n", state.String())

// Get circuit breaker metrics
metrics := cb.GetMetrics()
fmt.Printf("Error rate: %.2f%%\n", metrics.ErrorRate*100)

// Get rate limiter metrics
limiterMetrics := limiter.GetMetrics()
fmt.Printf("Allowed/Denied: %d/%d\n", 
    limiterMetrics.Allowed, limiterMetrics.Denied)

// Check current rate limit
limit, _ := limiter.GetLimit(ctx, "service:method")
fmt.Printf("Current limit: %.0f req/sec\n", limit.Rate)
```

## Performance Considerations

### Memory Usage
- Each rate limiter key uses ~100 bytes in local storage
- Each circuit breaker uses ~1KB of memory
- Consider using Redis for distributed deployments

### CPU Usage
- Rate limiting operations are O(1) for most operations
- Circuit breaker state management is lightweight
- Dynamic rate adjustment adds minimal overhead

### Network Impact
- Redis storage backend adds network latency
- Consider local storage for high-throughput services
- Use connection pooling for Redis connections

## Migration Guide

### From Previous Versions
1. Update configuration files with new sections
2. Initialize rate limiters and circuit breakers in service startup
3. Update client and server creation with new options
4. Monitor metrics during migration
5. Adjust configurations based on observed behavior

### Gradual Rollout
1. Start with monitoring only (no enforcement)
2. Enable rate limiting with high limits
3. Gradually tighten limits based on metrics
4. Enable circuit breaking with high thresholds
5. Adjust thresholds based on error rates

## API Reference

### Rate Limiter Interface
```go
type Limiter interface {
    Allow(ctx context.Context, key string) (bool, error)
    AllowN(ctx context.Context, key string, n int) (bool, error)
    Wait(ctx context.Context, key string) error
    WaitN(ctx context.Context, key string, n int) error
    Reserve(ctx context.Context, key string) (*Reservation, error)
    ReserveN(ctx context.Context, key string, n int) (*Reservation, error)
    UpdateLimit(ctx context.Context, key string, limit Limit) error
    GetLimit(ctx context.Context, key string) (Limit, error)
    Close() error
}
```

### Circuit Breaker Interface
```go
type CircuitBreaker interface {
    Execute(ctx context.Context, operation func() (interface{}, error)) (interface{}, error)
    AllowRequest(ctx context.Context) bool
    RecordSuccess()
    RecordFailure(err error)
    GetState() State
    GetMetrics() Metrics
    Reset()
    Close() error
}
```

### Manager Interface
```go
type Manager struct {
    // Methods for combined management
    Allow(ctx context.Context, service, method string) (bool, error)
    Execute(ctx context.Context, service, method string, operation func() (interface{}, error)) (interface{}, error)
    SetRateLimit(ctx context.Context, service, method string, limit Limit) error
    SetCircuitBreaker(service, method string, config circuitbreaker.Config) error
    GetMetrics() *ManagerMetrics
    Close() error
}
```

## Examples

Complete examples are available in:
- `example/ratelimit_circuitbreaker_example.go`
- `example/config/example.xml`
- Integration examples in client and server packages

## Support

For issues, questions, or feature requests:
1. Check the documentation
2. Review example code
3. Open an issue on GitHub
4. Contact the development team

## License

Rate limiting and circuit breaking features are part of the GFFG framework and are subject to the same licensing terms.