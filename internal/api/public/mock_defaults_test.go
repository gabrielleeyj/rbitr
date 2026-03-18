package public

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/gabrielleeyj/rbitr/internal/store"
)

func newPublicStoreMock(t *testing.T) *store.MockStoreAPI {
	t.Helper()
	mockStore := store.NewMockStoreAPI(t)
	addPublicFeatureFlagDefaults(mockStore)
	return mockStore
}

func addPublicFeatureFlagDefaults(mockStore *store.MockStoreAPI) {
	mockStore.On("GetDisableXTenantKey", mock.Anything).Maybe().Return(false, store.ErrNotFound)
	mockStore.On("GetFeatureRateLimiting", mock.Anything).Maybe().Return(false, store.ErrNotFound)
	mockStore.On("GetFeatureArgConstraints", mock.Anything).Maybe().Return(false, store.ErrNotFound)
	mockStore.On("GetFeatureFileGovernance", mock.Anything).Maybe().Return(false, store.ErrNotFound)
	mockStore.On("GetFeatureSessionTokens", mock.Anything).Maybe().Return(false, store.ErrNotFound)
}
