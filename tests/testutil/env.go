package testutil

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// LoadTestEnv loads environment variables from tests/fixtures/test.env
func LoadTestEnv() error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to get caller info")
	}

	testutilDir := filepath.Dir(filename)
	testsDir := filepath.Dir(testutilDir)
	projectRoot := filepath.Dir(testsDir)
	envPath := filepath.Join(projectRoot, ".env.test")

	return godotenv.Overload(envPath)
}

// INTEGRATION_TESTS=true go test -v ./tests/integration/ -run TestHappyPath/single
