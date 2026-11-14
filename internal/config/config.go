package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/natanzaum/goexpert-rate-limiter/internal/domain"
)

// RedisConfig holds the Redis connection settings.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// Config is the full application configuration.
type Config struct {
	Port        string
	Redis       RedisConfig
	Limiter     domain.Config
	TokenHeader string
}

// Load reads the configuration from environment variables, optionally
// loading a .env file located in the working directory.
func Load() (*Config, error) {
	// A missing .env file is fine: Docker and CI inject variables directly.
	_ = godotenv.Load()

	ipLimit, err := envInt("RATE_LIMIT_IP", 10)
	if err != nil {
		return nil, err
	}
	if ipLimit <= 0 {
		return nil, errors.New("RATE_LIMIT_IP must be greater than zero")
	}

	blockTime, err := envDuration("RATE_LIMIT_BLOCK_TIME", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	if blockTime <= 0 {
		return nil, errors.New("RATE_LIMIT_BLOCK_TIME must be greater than zero")
	}

	tokens, err := parseTokens(getEnv("RATE_LIMIT_TOKENS", "{}"))
	if err != nil {
		return nil, err
	}

	tokenHeader := strings.TrimSpace(getEnv("API_KEY_HEADER", "API_KEY"))
	if tokenHeader == "" {
		return nil, errors.New("API_KEY_HEADER must not be empty")
	}

	redisDB, err := envInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port: strings.TrimSpace(getEnv("PORT", "8080")),
		Redis: RedisConfig{
			Addr:     strings.TrimSpace(getEnv("REDIS_ADDR", "localhost:6379")),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		Limiter: domain.Config{
			IPLimit:   ipLimit,
			BlockTime: blockTime,
			Tokens:    tokens,
		},
		TokenHeader: tokenHeader,
	}, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration (e.g. 5m): %w", key, err)
	}
	return value, nil
}

// parseTokens accepts a JSON object such as {"token-a":100,"token-b":50}.
func parseTokens(raw string) (map[string]int, error) {
	tokens := make(map[string]int)
	if strings.TrimSpace(raw) == "" {
		return tokens, nil
	}

	if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
		return nil, fmt.Errorf("RATE_LIMIT_TOKENS must be a valid JSON object: %w", err)
	}

	for token, limit := range tokens {
		if strings.TrimSpace(token) == "" {
			return nil, errors.New("RATE_LIMIT_TOKENS contains an empty token")
		}
		if limit <= 0 {
			return nil, fmt.Errorf("token %q must have a limit greater than zero", token)
		}
	}

	return tokens, nil
}
