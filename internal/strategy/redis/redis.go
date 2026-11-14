package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/natanzaum/goexpert-rate-limiter/internal/config"
	"github.com/natanzaum/goexpert-rate-limiter/internal/domain"
)

const (
	blockKeyPrefix = "ratelimit:block:"
	countKeyPrefix = "ratelimit:count:"
	window         = time.Second
)

// consumeScript atomically checks the block, increments the current window
// counter and blocks the key when the limit is exceeded.
var consumeScript = redis.NewScript(`
local blockKey = KEYS[1]
local countKey = KEYS[2]
local limit = tonumber(ARGV[1])
local blockTtl = tonumber(ARGV[2])
local windowTtl = tonumber(ARGV[3])

local blockRemaining = redis.call('PTTL', blockKey)
if blockRemaining > 0 then
	return {0, 0, blockRemaining}
end

local count = redis.call('INCR', countKey)
if count == 1 then
	redis.call('PEXPIRE', countKey, windowTtl)
end

if count > limit then
	redis.call('SET', blockKey, '1', 'PX', blockTtl)
	return {0, 0, blockTtl}
end

local windowRemaining = redis.call('PTTL', countKey)
return {1, count, windowRemaining}
`)

// Store is the Redis implementation of domain.RateLimitStore.
type Store struct {
	client *redis.Client
}

// New creates a Redis-backed store and verifies the connection.
func New(cfg config.RedisConfig) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to redis at %s: %w", cfg.Addr, err)
	}

	return NewWithClient(client), nil
}

// NewWithClient creates a store from an existing client (useful in tests).
func NewWithClient(client *redis.Client) *Store {
	return &Store{client: client}
}

// Close releases the Redis connection.
func (s *Store) Close() error {
	return s.client.Close()
}

// Consume implements domain.RateLimitStore.
func (s *Store) Consume(ctx context.Context, key string, limit int, blockFor time.Duration) (domain.Decision, error) {
	if limit <= 0 {
		return domain.Decision{}, fmt.Errorf("redis store: limit must be positive, got %d", limit)
	}
	if blockFor <= 0 {
		return domain.Decision{}, fmt.Errorf("redis store: block duration must be positive, got %s", blockFor)
	}

	raw, err := consumeScript.Run(
		ctx,
		s.client,
		[]string{blockKeyPrefix + key, countKeyPrefix + key},
		limit,
		blockFor.Milliseconds(),
		window.Milliseconds(),
	).Result()
	if err != nil {
		return domain.Decision{}, fmt.Errorf("run rate limit script: %w", err)
	}

	values, ok := raw.([]interface{})
	if !ok || len(values) != 3 {
		return domain.Decision{}, fmt.Errorf("redis store: unexpected script result %v", raw)
	}

	allowed, err := toInt64(values[0])
	if err != nil {
		return domain.Decision{}, fmt.Errorf("redis store: parse allowed flag: %w", err)
	}
	count, err := toInt64(values[1])
	if err != nil {
		return domain.Decision{}, fmt.Errorf("redis store: parse counter: %w", err)
	}
	retryMillis, err := toInt64(values[2])
	if err != nil {
		return domain.Decision{}, fmt.Errorf("redis store: parse retry-after: %w", err)
	}

	if allowed == 1 {
		remaining := int(limit) - int(count)
		if remaining < 0 {
			remaining = 0
		}
		return domain.Decision{
			Allowed:    true,
			Remaining:  remaining,
			RetryAfter: time.Duration(retryMillis) * time.Millisecond,
		}, nil
	}

	return domain.Decision{
		Allowed:    false,
		RetryAfter: time.Duration(retryMillis) * time.Millisecond,
	}, nil
}

func toInt64(v interface{}) (int64, error) {
	switch value := v.(type) {
	case int64:
		return value, nil
	case int:
		return int64(value), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}
