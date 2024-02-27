//go:build !ut

// This file contains tests that require a real RisingWave Cloud endpoint.

package cloudsdk

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	TestEndpoint  = os.Getenv("RWC_ENDPOINT")
	TestAPIKey    = os.Getenv("RWC_API_KEY")
	TestAPISecret = os.Getenv("RWC_API_SECRET")
)

func getTestAccountServiceClient(t *testing.T) AccountServiceClientInterface {
	t.Helper()
	client, err := NewAccountServiceClient(context.Background(), TestEndpoint, TestAPIKey, TestAPISecret)
	require.NoError(t, err)
	return client
}

func TestPing(t *testing.T) {
	client := getTestAccountServiceClient(t)
	err := client.Ping(context.Background())
	assert.NoError(t, err)
}
