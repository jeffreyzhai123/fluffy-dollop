// tests/testutil/redis.go
package testutil

import (
	"context"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

// GetStreamLength returns the current length of a Redis stream
// Returns 0 if stream doesn't exist (not an error - empty streams are deleted)
func GetStreamLength(client *redis.Client, stream string) (int64, error) {
	length, err := client.XLen(context.Background(), stream).Result()
	if err != nil {
		// If stream doesn't exist, that's fine - it means it's empty
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return length, nil
}

// GetPendingCount returns the number of pending (unacked) messages
// Returns 0 if group/stream doesn't exist (not an error)
func GetPendingCount(client *redis.Client, stream, group string) (int64, error) {
	info, err := client.XPending(context.Background(), stream, group).Result()
	if err != nil {
		// Stream/group doesn't exist - that's fine, means nothing pending
		if err == redis.Nil {
			return 0, nil
		}
		// Check for NOGROUP error
		if strings.Contains(err.Error(), "NOGROUP") {
			return 0, nil
		}
		return 0, err
	}
	return info.Count, nil
}

// InjectMalformedMessage adds invalid JSON to stream for testing error handling
func InjectMalformedMessage(t *testing.T, client *redis.Client, stream string) string {
	t.Helper()
	id, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"job": "this is not valid json {{{",
		},
	}).Result()
	if err != nil {
		t.Fatalf("failed to inject malformed message: %v", err)
	}
	return id
}
