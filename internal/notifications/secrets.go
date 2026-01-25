package notifications

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var ErrSecretNotFound = errors.New("secret ref not found")

type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

type SecretProvider interface {
	Match(ref string) bool
	Resolve(ctx context.Context, ref string) (string, error)
}

type cacheItem struct {
	value     string
	expiresAt time.Time
}

type CompositeResolver struct {
	providers []SecretProvider
	ttl       time.Duration
	mu        sync.RWMutex
	cache     map[string]cacheItem
}

func NewCompositeResolver(providers []SecretProvider, ttl time.Duration) *CompositeResolver {
	return &CompositeResolver{
		providers: providers,
		ttl:       ttl,
		cache:     make(map[string]cacheItem),
	}
}

func (r *CompositeResolver) Resolve(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", ErrSecretNotFound
	}

	now := time.Now()
	r.mu.RLock()
	if cached, ok := r.cache[ref]; ok && now.Before(cached.expiresAt) {
		r.mu.RUnlock()
		return cached.value, nil
	}
	r.mu.RUnlock()

	for _, provider := range r.providers {
		if !provider.Match(ref) {
			continue
		}
		value, err := provider.Resolve(ctx, ref)
		if err != nil {
			return "", err
		}
		r.mu.Lock()
		r.cache[ref] = cacheItem{value: value, expiresAt: now.Add(r.ttl)}
		r.mu.Unlock()
		return value, nil
	}

	return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
}

type EnvProvider struct{}

func (p EnvProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, "env://")
}

func (p EnvProvider) Resolve(ctx context.Context, ref string) (string, error) {
	_ = ctx
	key := strings.TrimPrefix(ref, "env://")
	if key == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	return value, nil
}

type FileProvider struct{}

func (p FileProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, "file://")
}

func (p FileProvider) Resolve(ctx context.Context, ref string) (string, error) {
	_ = ctx
	path := strings.TrimPrefix(ref, "file://")
	if path == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	return strings.TrimSpace(string(data)), nil
}

func redactRef(ref string) string {
	const maxPrefix = 12
	if len(ref) <= maxPrefix {
		return ref
	}
	return ref[:maxPrefix] + "..."
}
