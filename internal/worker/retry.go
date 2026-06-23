package worker

import (
	"context"
	"errors"
	"strings"
	"time"
)

type RetryConfig struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		BaseDelay: time.Second,
		MaxDelay:  30 * time.Second,
	}
}

func retryDelay(cfg RetryConfig, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := cfg.BaseDelay
	for range attempts - 1 {
		if delay >= cfg.MaxDelay {
			return cfg.MaxDelay
		}
		delay *= 2
	}
	if delay > cfg.MaxDelay {
		return cfg.MaxDelay
	}
	return delay
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "must be"),
		strings.Contains(msg, "unknown job type"),
		strings.Contains(msg, "is invalid"):
		return false
	case strings.Contains(msg, "fetch request failed"),
		strings.Contains(msg, "fetch read body failed"):
		return true
	default:
		return false
	}
}
