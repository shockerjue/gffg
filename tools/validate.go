package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/zzlog"
)

// ConfigValidatorTool provides configuration validation functionality
type ConfigValidatorTool struct {
	configPath string
	validator  *common.ConfigValidator
}

// NewConfigValidatorTool creates a new configuration validator
func NewConfigValidatorTool(configPath string) *ConfigValidatorTool {
	return &ConfigValidatorTool{
		configPath: configPath,
		validator:  common.NewConfigValidator(),
	}
}

// Validate performs comprehensive configuration validation
func (c *ConfigValidatorTool) Validate() error {
	// Check if config file exists
	if err := c.validateFileExists(); err != nil {
		return err
	}

	// Load configuration
	config.Init(c.configPath)

	// Validate server configuration
	c.validateServerConfig()

	// Validate client configuration
	c.validateClientConfig()

	// Validate metrics configuration
	c.validateMetricsConfig()

	// Validate registry configuration
	c.validateRegistryConfig()

	// Validate logging configuration
	c.validateLogConfig()

	// Validate security configuration
	c.validateSecurityConfig()

	// Validate monitoring configuration
	c.validateMonitoringConfig()

	// Check for validation errors
	if c.validator.HasErrors() {
		return fmt.Errorf(c.validator.GetErrorString())
	}

	return nil
}

// validateFileExists checks if the configuration file exists
func (c *ConfigValidatorTool) validateFileExists() error {
	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		return fmt.Errorf("configuration file does not exist: %s", c.configPath)
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(c.configPath))
	if ext != ".xml" {
		return fmt.Errorf("unsupported configuration file format: %s (only .xml files are supported)", ext)
	}

	// Check file size
	info, err := os.Stat(c.configPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if info.Size() == 0 {
		return fmt.Errorf("configuration file is empty: %s", c.configPath)
	}

	if info.Size() > 10*1024*1024 { // 10MB
		return fmt.Errorf("configuration file is too large: %d bytes (max 10MB)", info.Size())
	}

	return nil
}

// validateServerConfig validates server configuration
func (c *ConfigValidatorTool) validateServerConfig() {
	// Validate port
	port := config.Get("server", "port").Int(0)
	c.validator.ValidatePort("server.port", port)

	// Validate coroutines
	coroutines := config.Get("server", "coroutines").Int(32)
	c.validator.ValidatePoolSize("server.coroutines", coroutines)

	// Validate channels
	channels := config.Get("server", "channels").Int(10000)
	c.validator.ValidateBufferSize("server.channels", channels)

	// Validate timeout settings
	readTimeout := config.Get("server", "timeout", "read").String("30s")
	if dur, err := time.ParseDuration(readTimeout); err == nil {
		c.validator.ValidateTimeout("server.timeout.read", dur)
	}

	writeTimeout := config.Get("server", "timeout", "write").String("30s")
	if dur, err := time.ParseDuration(writeTimeout); err == nil {
		c.validator.ValidateTimeout("server.timeout.write", dur)
	}

	// Validate rate limiting
	if config.Get("server", "rate_limit", "enabled").Bool() {
		rateLimit := config.Get("server", "rate_limit", "global").Int(1000)
		burst := config.Get("server", "rate_limit", "burst").Int(200)
		c.validator.ValidateRateLimit("server.rate_limit", rateLimit, burst)
	}

	// Validate service name and group
	name := config.Get("server", "name").String("")
	if name == "" {
		c.validator.Validate("server.name", name, func(val interface{}) error {
			return fmt.Errorf("server name cannot be empty")
		})
	}

	group := config.Get("server", "group").String("")
	if group == "" {
		c.validator.Validate("server.group", group, func(val interface{}) error {
			return fmt.Errorf("server group cannot be empty")
		})
	}
}

// validateClientConfig validates client configuration
func (c *ConfigValidatorTool) validateClientConfig() {
	// Validate connection pool size
	poolSize := config.Get("client", "pool", "size").Int(8)
	c.validator.ValidatePoolSize("client.pool.size", poolSize)

	// Validate timeout settings
	connectTimeout := config.Get("client", "timeout", "connect").String("5s")
	if dur, err := time.ParseDuration(connectTimeout); err == nil {
		c.validator.ValidateTimeout("client.timeout.connect", dur)
	}

	callTimeout := config.Get("client", "timeout", "call").String("60s")
	if dur, err := time.ParseDuration(callTimeout); err == nil {
		c.validator.ValidateTimeout("client.timeout.call", dur)
	}

	// Validate retry configuration
	maxAttempts := config.Get("client", "retry", "max_attempts").Int(3)
	c.validator.Validate("client.retry.max_attempts", maxAttempts, func(val interface{}) error {
		return common.ValidateRetryConfig(val.(int), 0)
	})

	initialBackoff := config.Get("client", "retry", "initial_backoff").String("100ms")
	if dur, err := time.ParseDuration(initialBackoff); err == nil {
		c.validator.ValidateTimeout("client.retry.initial_backoff", dur)
	}
}

// validateMetricsConfig validates metrics configuration
func (c *ConfigValidatorTool) validateMetricsConfig() {
	if !config.Get("metrics", "enabled").Bool() {
		return
	}

	// Validate batch size
	batchSize := config.Get("metrics", "batch_size").Int(1000)
	c.validator.ValidateBufferSize("metrics.batch_size", batchSize)

	// Validate channel size
	channelSize := config.Get("metrics", "channel_size").Int(10000)
	c.validator.ValidateBufferSize("metrics.channel_size", channelSize)

	// Validate interval
	interval := config.Get("metrics", "interval").String("1s")
	if dur, err := time.ParseDuration(interval); err == nil {
		c.validator.ValidateTimeout("metrics.interval", dur)
	}

	// Validate Kafka brokers
	brokers := config.Get("metrics", "brokers").String("")
	if brokers == "" {
		c.validator.Validate("metrics.brokers", brokers, func(val interface{}) error {
			return fmt.Errorf("metrics brokers cannot be empty when metrics is enabled")
		})
	}
}

// validateRegistryConfig validates registry configuration
func (c *ConfigValidatorTool) validateRegistryConfig() {
	// Validate registry address
	address := config.Get("registry", "address").String("")
	if address == "" {
		c.validator.Validate("registry.address", address, func(val interface{}) error {
			return fmt.Errorf("registry address cannot be empty")
		})
	}

	// Validate timeout
	timeout := config.Get("registry", "timeout").String("5s")
	if dur, err := time.ParseDuration(timeout); err == nil {
		c.validator.ValidateTimeout("registry.timeout", dur)
	}

	// Validate refresh interval
	refreshInterval := config.Get("registry", "refresh_interval").String("30s")
	if dur, err := time.ParseDuration(refreshInterval); err == nil {
		c.validator.ValidateTimeout("registry.refresh_interval", dur)
	}

	// Validate circuit breaker configuration
	if config.Get("registry", "circuit_breaker", "enabled").Bool() {
		errorThreshold := config.Get("registry", "circuit_breaker", "error_threshold").Int(50)
		c.validator.Validate("registry.circuit_breaker.error_threshold", errorThreshold, func(val interface{}) error {
			v := val.(int)
			if v < 1 || v > 100 {
				return fmt.Errorf("error threshold must be between 1 and 100")
			}
			return nil
		})

		sleepWindow := config.Get("registry", "circuit_breaker", "sleep_window").String("30s")
		if dur, err := time.ParseDuration(sleepWindow); err == nil {
			c.validator.ValidateTimeout("registry.circuit_breaker.sleep_window", dur)
		}
	}
}

// validateLogConfig validates logging configuration
func (c *ConfigValidatorTool) validateLogConfig() {
	logFile := config.Get("log", "log_file").String("")
	if logFile != "" {
		// Check if log directory is writable
		logDir := filepath.Dir(logFile)
		if err := isWritable(logDir); err != nil {
			c.validator.Validate("log.log_file", logFile, func(val interface{}) error {
				return fmt.Errorf("log directory is not writable: %v", err)
			})
		}
	}

	// Validate log level
	level := config.Get("log", "level").String("info")
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "panic": true, "fatal": true,
	}
	if !validLevels[strings.ToLower(level)] {
		c.validator.Validate("log.level", level, func(val interface{}) error {
			return fmt.Errorf("invalid log level: %s (valid levels: debug, info, warn, error, panic, fatal)", level)
		})
	}

	// Validate log rotation settings
	maxSize := config.Get("log", "max_size").Int(100)
	if maxSize <= 0 || maxSize > 1000 {
		c.validator.Validate("log.max_size", maxSize, func(val interface{}) error {
			return fmt.Errorf("log max size must be between 1 and 1000 MB")
		})
	}

	maxBackups := config.Get("log", "max_backups").Int(10)
	if maxBackups < 0 || maxBackups > 100 {
		c.validator.Validate("log.max_backups", maxBackups, func(val interface{}) error {
			return fmt.Errorf("log max backups must be between 0 and 100")
		})
	}

	maxAge := config.Get("log", "max_age").Int(30)
	if maxAge <= 0 || maxAge > 365 {
		c.validator.Validate("log.max_age", maxAge, func(val interface{}) error {
			return fmt.Errorf("log max age must be between 1 and 365 days")
		})
	}
}

// validateSecurityConfig validates security configuration
func (c *ConfigValidatorTool) validateSecurityConfig() {
	if config.Get("security", "tls", "enabled").Bool() {
		certFile := config.Get("security", "tls", "cert_file").String("")
		if certFile == "" {
			c.validator.Validate("security.tls.cert_file", certFile, func(val interface{}) error {
				return fmt.Errorf("TLS certificate file is required when TLS is enabled")
			})
		} else if !fileExists(certFile) {
			c.validator.Validate("security.tls.cert_file", certFile, func(val interface{}) error {
				return fmt.Errorf("TLS certificate file does not exist: %s", certFile)
			})
		}

		keyFile := config.Get("security", "tls", "key_file").String("")
		if keyFile == "" {
			c.validator.Validate("security.tls.key_file", keyFile, func(val interface{}) error {
				return fmt.Errorf("TLS key file is required when TLS is enabled")
			})
		} else if !fileExists(keyFile) {
			c.validator.Validate("security.tls.key_file", keyFile, func(val interface{}) error {
				return fmt.Errorf("TLS key file does not exist: %s", keyFile)
			})
		}
	}

	if config.Get("security", "auth", "enabled").Bool() {
		secret := config.Get("security", "auth", "secret").String("")
		if secret == "" {
			c.validator.Validate("security.auth.secret", secret, func(val interface{}) error {
				return fmt.Errorf("authentication secret is required when auth is enabled")
			})
		}

		expiry := config.Get("security", "auth", "expiry").String("24h")
		if dur, err := time.ParseDuration(expiry); err == nil {
			c.validator.ValidateTimeout("security.auth.expiry", dur)
		}
	}
}

// validateMonitoringConfig validates monitoring configuration
func (c *ConfigValidatorTool) validateMonitoringConfig() {
	if config.Get("monitoring", "prometheus", "enabled").Bool() {
		port := config.Get("monitoring", "prometheus", "port").Int(9091)
		c.validator.ValidatePort("monitoring.prometheus.port", port)
	}

	if config.Get("monitoring", "health", "enabled").Bool() {
		port := config.Get("monitoring", "health", "port").Int(9092)
		c.validator.ValidatePort("monitoring.health.port", port)
	}

	if config.Get("monitoring", "pprof", "enabled").Bool() {
		port := config.Get("monitoring", "pprof", "port").Int(7777)
		c.validator.ValidatePort("monitoring.pprof.port", port)
	}
}

// RunValidation runs configuration validation and returns results
func RunValidation(configPath string) (bool, []string) {
	validator := NewConfigValidatorTool(configPath)

	// Initialize logger for validation output
	zzlog.Init(
		zzlog.WithLogName(""), // Console only
		zzlog.WithLevel("info"),
	)

	fmt.Printf("🔍 Validating configuration file: %s\n", configPath)

	if err := validator.Validate(); err != nil {
		fmt.Printf("❌ Configuration validation failed:\n%s\n", err)
		return false, validator.validator.GetErrors()
	}

	fmt.Printf("✅ Configuration validation passed!\n")

	// Print configuration summary
	validator.printConfigSummary()

	return true, nil
}

// printConfigSummary prints a summary of the configuration
func (c *ConfigValidatorTool) printConfigSummary() {
	fmt.Println("\n📋 Configuration Summary:")
	fmt.Println("========================")

	// Server configuration
	fmt.Printf("Server:\n")
	fmt.Printf("  Name: %s\n", config.Get("server", "name").String(""))
	fmt.Printf("  Group: %s\n", config.Get("server", "group").String(""))
	fmt.Printf("  Port: %d\n", config.Get("server", "port").Int(0))
	fmt.Printf("  Coroutines: %d\n", config.Get("server", "coroutines").Int(32))
	fmt.Printf("  Channels: %d\n", config.Get("server", "channels").Int(10000))

	// Registry configuration
	fmt.Printf("\nRegistry:\n")
	fmt.Printf("  Type: %s\n", config.Get("registry", "type").String("polaris"))
	fmt.Printf("  Address: %s\n", config.Get("registry", "address").String(""))

	// Metrics configuration
	if config.Get("metrics", "enabled").Bool() {
		fmt.Printf("\nMetrics:\n")
		fmt.Printf("  Enabled: true\n")
		fmt.Printf("  Brokers: %s\n", config.Get("metrics", "brokers").String(""))
		fmt.Printf("  Batch Size: %d\n", config.Get("metrics", "batch_size").Int(1000))
	}

	// Logging configuration
	fmt.Printf("\nLogging:\n")
	fmt.Printf("  Level: %s\n", config.Get("log", "level").String("info"))
	logFile := config.Get("log", "log_file").String("")
	if logFile != "" {
		fmt.Printf("  File: %s\n", logFile)
	}

	fmt.Println("========================")
}

// HealthCheck performs a health check on the configuration
func (c *ConfigValidatorTool) HealthCheck(ctx context.Context) error {
	// Create health checker
	healthChecker := common.NewHealthChecker()

	// Add configuration file health check
	healthChecker.Register(&common.HealthCheck{
		Name:        "config_file",
		Description: "Configuration file accessibility",
		Check: func(ctx context.Context) error {
			return c.validateFileExists()
		},
		Timeout:  5 * time.Second,
		Interval: 30 * time.Second,
		Critical: true,
	})

	// Add configuration validation health check
	healthChecker.Register(&common.HealthCheck{
		Name:        "config_validation",
		Description: "Configuration validation",
		Check: func(ctx context.Context) error {
			return c.Validate()
		},
		Timeout:  10 * time.Second,
		Interval: 60 * time.Second,
		Critical: true,
	})

	// Run health checks
	healthChecker.Run()
	defer healthChecker.Stop()

	// Wait for initial health check
	time.Sleep(1 * time.Second)

	// Get overall status
	status := healthChecker.GetOverallStatus()
	if status != common.HealthStatusHealthy {
		report := healthChecker.GetStatusReport()
		return fmt.Errorf("configuration health check failed: %v", report)
	}

	return nil
}

// Helper functions

// fileExists checks if a file exists
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// isWritable checks if a directory is writable
func isWritable(path string) error {
	// Try to create a temporary file in the directory
	tempFile := filepath.Join(path, ".gffg_write_test")
	if err := os.WriteFile(tempFile, []byte("test"), 0644); err != nil {
		return err
	}
	os.Remove(tempFile)
	return nil
}

// ValidateConfig is the main entry point for configuration validation
func ValidateConfig(configPath string) {
	success, _ := RunValidation(configPath)
	if !success {
		os.Exit(1)
	}

	// If we get here, validation passed
	fmt.Println("\n🎉 Configuration is valid and ready to use!")

	// Provide next steps
	fmt.Println("\nNext steps:")
	fmt.Println("1. Start the server: go run main.go -config " + configPath)
	fmt.Println("2. Check logs for startup information")
	fmt.Println("3. Use the health endpoint to verify service status")
}
