package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// WaitFor polls condition until true or timeout
// Uses testing.TB interface to work with *testing.T, *testing.B, and mocks
func WaitFor(t *testing.T, timeout time.Duration, condition func() bool, msgAndArgs ...interface{}) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			msg := "condition not met within timeout"
			if len(msgAndArgs) > 0 {
				msg = fmt.Sprintf("%v", msgAndArgs[0])
			}
			t.Fatalf("%s (timeout: %v)", msg, timeout)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// WaitForStreamLength polls until stream reaches expected length
// Handles errors by continuing to poll (transient errors shouldn't fail test)
func WaitForStreamLength(t *testing.T, client *redis.Client, stream string, expected int64, timeout time.Duration) {
	t.Helper()
	WaitFor(t, timeout,
		func() bool {
			length, err := GetStreamLength(client, stream)
			if err != nil {
				// Log the error but keep polling
				t.Logf("error getting stream length (will retry): %v", err)
				return false
			}
			return length == expected
		},
		fmt.Sprintf("stream %q to reach length %d", stream, expected),
	)
}

// WaitForStreamLengthAtLeast polls until stream reaches at least the expected length
func WaitForStreamLengthAtLeast(t *testing.T, client *redis.Client, stream string, minExpected int64, timeout time.Duration) {
	t.Helper()
	WaitFor(t, timeout,
		func() bool {
			length, err := GetStreamLength(client, stream)
			if err != nil {
				t.Logf("error getting stream length (will retry): %v", err)
				return false
			}
			return length >= minExpected
		},
		fmt.Sprintf("stream %q to reach at least length %d", stream, minExpected),
	)
}

// WaitForPendingCount polls until pending messages reach expected count
func WaitForPendingCount(t *testing.T, client *redis.Client, stream, group string, expected int64, timeout time.Duration) {
	t.Helper()
	WaitFor(t, timeout,
		func() bool {
			count, err := GetPendingCount(client, stream, group)
			if err != nil {
				t.Logf("error getting pending count (will retry): %v", err)
				return false
			}
			return count == expected
		},
		fmt.Sprintf("pending count in stream %q group %q to reach %d", stream, group, expected),
	)
}

// WaitForWorkerShutdown waits for worker done channel to close or return
func WaitForWorkerShutdown(t *testing.T, done <-chan error, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		t.Fatalf("worker did not shut down within %v", timeout)
		return nil
	}
}
