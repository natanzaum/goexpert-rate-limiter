package domain

import (
	"context"
	"fmt"
	"time"
)

// Scope identifies which rule was applied to a request.
type Scope string

const (
	ScopeIP    Scope = "ip"
	ScopeToken Scope = "token"
)

// Config holds the rate limiting business rules.
type Config struct {
	// IPLimit is the maximum number of requests per second per IP.
	IPLimit int
	// BlockTime is how long an offending IP/token stays blocked.
	BlockTime time.Duration
	// Tokens maps an access token to its own requests-per-second limit.
	Tokens map[string]int
}

// Decision is the outcome of a rate limit check.
type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

// RateLimitStore is the persistence strategy used by the limiter.
//
// Implementations must perform the counter increment and the block check
// atomically, so that concurrent requests cannot exceed the configured limit.
type RateLimitStore interface {
	Consume(ctx context.Context, key string, limit int, blockFor time.Duration) (Decision, error)
}

// Result carries the decision plus the rule that produced it.
type Result struct {
	Scope    Scope
	Key      string
	Limit    int
	Decision Decision
}

// Service contains the rate limiting business rules. It is independent from
// HTTP and from any concrete persistence implementation.
type Service struct {
	store  RateLimitStore
	config Config
}

// NewService creates a rate limiter service backed by the given store.
func NewService(store RateLimitStore, config Config) *Service {
	return &Service{store: store, config: config}
}

// Resolve chooses the rule that applies to a request.
//
// A configured token always takes precedence over the global IP limit. When
// the token is missing or unknown, the IP rule is used.
func (s *Service) Resolve(ip, token string) (key string, limit int, scope Scope) {
	if token != "" {
		if tokenLimit, ok := s.config.Tokens[token]; ok && tokenLimit > 0 {
			return "token:" + token, tokenLimit, ScopeToken
		}
	}
	return "ip:" + ip, s.config.IPLimit, ScopeIP
}

// Check consumes one request for the resolved key.
func (s *Service) Check(ctx context.Context, ip, token string) (Result, error) {
	key, limit, scope := s.Resolve(ip, token)

	decision, err := s.store.Consume(ctx, key, limit, s.config.BlockTime)
	if err != nil {
		return Result{}, fmt.Errorf("consume rate limit key %q: %w", key, err)
	}

	return Result{Scope: scope, Key: key, Limit: limit, Decision: decision}, nil
}
