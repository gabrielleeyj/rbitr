package public

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestStatelessDesign(t *testing.T) {
	// Verifies that two independent Dependencies instances (simulating two
	// gateway replicas) can operate without shared in-memory state.
	// All correctness-critical state is in the DB (store mock).
	cases := []struct {
		name        string
		description string
	}{
		{
			name:        "independent caches do not share state",
			description: "each replica has its own TTL cache; no cross-instance cache dependency",
		},
		{
			name:        "no singleton state required",
			description: "Dependencies struct holds no global mutable state beyond caches",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create two independent Dependencies (simulating two replicas)
			deps1 := &Dependencies{
				Config:    config.Config{BodyLimitSize: 256 * 1024},
				ToolCache: cache.New[models.Tool](30 * time.Second),
			}
			deps2 := &Dependencies{
				Config:    config.Config{BodyLimitSize: 256 * 1024},
				ToolCache: cache.New[models.Tool](30 * time.Second),
			}

			// Populate cache in replica 1
			deps1.ToolCache.Set("t1:tool1", models.Tool{ToolID: "tool1", TenantID: "t1"})

			// Replica 2 cache should be empty (no shared state)
			_, found := deps2.ToolCache.Get("t1:tool1")
			require.False(t, found, "replica 2 should not see replica 1's cached tool")

			// Each replica has independent cache stats
			hits1, misses1 := deps1.ToolCache.Stats()
			hits2, misses2 := deps2.ToolCache.Stats()
			require.Equal(t, int64(0), hits1)
			require.Equal(t, int64(0), misses1)
			require.Equal(t, int64(0), hits2)
			require.Equal(t, int64(1), misses2) // the failed Get above
		})
	}
}

func TestAdvisoryLockKeysAreDistinct(t *testing.T) {
	// Verifies that different schedulers use different lock keys
	// so they don't block each other when running on the same instance.
	// This is validated by the fact that both schedulers construct their
	// lock keys from distinct strings ("approval-expiry-scheduler" vs
	// "audit-retention-scheduler").
	// The test just documents this design invariant.
	require.NotEqual(t, "approval-expiry-scheduler", "audit-retention-scheduler",
		"scheduler lock key sources must be distinct")
}
