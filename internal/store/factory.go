package store

import (
	"context"
	"fmt"
	"gpt-load/internal/types"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// NewStore creates a new store based on the application configuration.
func NewStore(cfg types.ConfigManager) (Store, error) {
	redisDSN := cfg.GetRedisDSN()
	if redisDSN != "" {
		opts, err := redis.ParseURL(redisDSN)
		if err != nil {
			return nil, fmt.Errorf("failed to parse redis DSN: %w", err)
		}

		client := redis.NewClient(opts)
		if err := client.Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("failed to connect to redis: %w", err)
		}

		logrus.Debug("Successfully connected to Redis.")
		return NewRedisStore(client), nil
	}

	logrus.Info("Redis DSN not configured, falling back to in-memory store.")
	return NewMemoryStore(), nil
}
