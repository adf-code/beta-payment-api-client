package redis

import (
	"beta-payment-api-client/config"
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"sync"
)

type RedisClient struct {
	redisHost string
	redisPort string
	redisPass string
	client    *redis.Client
	logger    zerolog.Logger
	once      sync.Once
}

func NewRedisClient(cfg *config.AppConfig, logger zerolog.Logger) *RedisClient {
	return &RedisClient{
		redisHost: cfg.RedisHost,
		redisPort: cfg.RedisPort,
		redisPass: cfg.RedisPassword,
		logger:    logger,
	}
}

func (r *RedisClient) InitRedis() *redis.Client {
	r.once.Do(func() {
		addr := fmt.Sprintf("%s:%s", r.redisHost, r.redisPort)

		r.client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: r.redisPass, // 🔐 add password if set
			DB:       0,           // default DB
		})

		if err := r.client.Ping(context.Background()).Err(); err != nil {
			r.logger.Fatal().Err(err).Msg("❌ Failed to connect Redis")
		}

		r.logger.Info().Msgf("✅ Connected to Redis at %s", addr)
	})

	return r.client
}
