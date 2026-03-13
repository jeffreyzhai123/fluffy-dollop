package testutil

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// UniqueName generates a unique name for test resources
// Format: prefix-TestName-randomNumber
func UniqueName(t *testing.T, prefix string) string {
	// Clean test name (remove slashes from subtests)
	testName := strings.ReplaceAll(t.Name(), "/", "-")
	testName = strings.ReplaceAll(testName, " ", "_")

	return fmt.Sprintf("%s-%s-%d", prefix, testName, rand.Int63())
}
