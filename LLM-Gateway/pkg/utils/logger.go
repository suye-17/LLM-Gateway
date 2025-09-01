// Package utils provides utility functions for the LLM Gateway
package utils

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with additional functionality
type Logger struct {
	*logrus.Logger
}

// NewLogger creates a new logger instance with specified configuration
func NewLogger(config *types.LoggingConfig) *Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Set log format
	if config.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	// Set output
	var output io.Writer = os.Stdout
	if config.Output == "stderr" {
		output = os.Stderr
	} else if config.Output != "" && config.Output != "stdout" {
		// File output
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			logger.WithError(err).Error("Failed to open log file, falling back to stdout")
			output = os.Stdout
		} else {
			output = file
		}
	}
	logger.SetOutput(output)

	return &Logger{Logger: logger}
}

// WithRequestID adds request ID to log context
func (l *Logger) WithRequestID(requestID string) *logrus.Entry {
	return l.WithField("request_id", requestID)
}

// WithUserID adds user ID to log context
func (l *Logger) WithUserID(userID string) *logrus.Entry {
	return l.WithField("user_id", userID)
}

// WithProvider adds provider information to log context
func (l *Logger) WithProvider(provider string) *logrus.Entry {
	return l.WithField("provider", provider)
}

// WithDuration adds duration to log context
func (l *Logger) WithDuration(duration time.Duration) *logrus.Entry {
	return l.WithField("duration_ms", duration.Milliseconds())
}

// WithHTTPRequest logs HTTP request details
func (l *Logger) WithHTTPRequest(method, path, userAgent, clientIP string) *logrus.Entry {
	return l.WithFields(logrus.Fields{
		"http_method":     method,
		"http_path":       path,
		"http_user_agent": userAgent,
		"client_ip":       clientIP,
	})
}

// WithError adds error information with additional context
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// LogAPIRequest logs API request with structured fields
func (l *Logger) LogAPIRequest(ctx context.Context, method, path, userAgent, clientIP, requestID string, startTime time.Time) {
	l.WithFields(logrus.Fields{
		"type":            "api_request",
		"request_id":      requestID,
		"http_method":     method,
		"http_path":       path,
		"http_user_agent": userAgent,
		"client_ip":       clientIP,
		"timestamp":       startTime.Format(time.RFC3339),
	}).Info("API request started")
}

// LogAPIResponse logs API response with structured fields
func (l *Logger) LogAPIResponse(ctx context.Context, requestID string, statusCode int, duration time.Duration, responseSize int64) {
	entry := l.WithFields(logrus.Fields{
		"type":          "api_response",
		"request_id":    requestID,
		"status_code":   statusCode,
		"duration_ms":   duration.Milliseconds(),
		"response_size": responseSize,
	})

	if statusCode >= 400 {
		entry.Warn("API request completed with error")
	} else {
		entry.Info("API request completed successfully")
	}
}

// LogProviderCall logs provider API calls
func (l *Logger) LogProviderCall(ctx context.Context, provider, model, requestID string, startTime time.Time) {
	l.WithFields(logrus.Fields{
		"type":       "provider_call",
		"request_id": requestID,
		"provider":   provider,
		"model":      model,
		"timestamp":  startTime.Format(time.RFC3339),
	}).Info("Provider API call started")
}

// LogProviderResponse logs provider API responses
func (l *Logger) LogProviderResponse(ctx context.Context, provider, requestID string, statusCode int, duration time.Duration, tokens int) {
	entry := l.WithFields(logrus.Fields{
		"type":        "provider_response",
		"request_id":  requestID,
		"provider":    provider,
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
		"tokens":      tokens,
	})

	if statusCode >= 400 {
		entry.Warn("Provider API call completed with error")
	} else {
		entry.Info("Provider API call completed successfully")
	}
}

// LogRateLimitExceeded logs rate limit violations
func (l *Logger) LogRateLimitExceeded(ctx context.Context, userID, apiKey, endpoint string) {
	l.WithFields(logrus.Fields{
		"type":     "rate_limit_exceeded",
		"user_id":  userID,
		"api_key":  maskAPIKey(apiKey),
		"endpoint": endpoint,
	}).Warn("Rate limit exceeded")
}

// LogAuthFailure logs authentication failures
func (l *Logger) LogAuthFailure(ctx context.Context, reason, clientIP, userAgent string) {
	l.WithFields(logrus.Fields{
		"type":            "auth_failure",
		"reason":          reason,
		"client_ip":       clientIP,
		"http_user_agent": userAgent,
	}).Warn("Authentication failed")
}

// LogSystemError logs system-level errors
func (l *Logger) LogSystemError(ctx context.Context, component string, err error, additionalFields map[string]interface{}) {
	fields := logrus.Fields{
		"type":      "system_error",
		"component": component,
		"error":     err.Error(),
	}

	// Add additional fields if provided
	for k, v := range additionalFields {
		fields[k] = v
	}

	l.WithFields(fields).Error("System error occurred")
}

// maskAPIKey masks API key for logging (shows only first 8 characters)
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:8] + "****"
}

// Global logger instance
var DefaultLogger *Logger

// InitDefaultLogger initializes the default logger
func InitDefaultLogger(config *types.LoggingConfig) {
	DefaultLogger = NewLogger(config)
}

// Convenience functions using the default logger
func Info(args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Info(args...)
	}
}

func Warn(args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Warn(args...)
	}
}

func Error(args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Error(args...)
	}
}

func Debug(args ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Debug(args...)
	}
}

func WithField(key string, value interface{}) *logrus.Entry {
	if DefaultLogger != nil {
		return DefaultLogger.WithField(key, value)
	}
	return logrus.NewEntry(logrus.StandardLogger())
}
