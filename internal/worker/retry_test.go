package worker

import (
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	cfg := RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second}

	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 10, want: 30 * time.Second},
	}

	for _, tc := range tests {
		if got := retryDelay(cfg, tc.attempts); got != tc.want {
			t.Fatalf("retryDelay(%d) = %s, want %s", tc.attempts, got, tc.want)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: errString("hash.text must be a non-empty string"), want: false},
		{err: errString("unknown job type \"nope\""), want: false},
		{err: errString("fetch.url is invalid: parse"), want: false},
		{err: errString("fetch request failed: connection refused"), want: true},
		{err: errString("fetch read body failed: EOF"), want: true},
	}

	for _, tc := range tests {
		if got := isRetryable(tc.err); got != tc.want {
			t.Fatalf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
