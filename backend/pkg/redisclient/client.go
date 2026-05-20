package redisclient

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
	"opinion-analysis/config"
)

func New(cfg config.RedisConfig) *redis.Client {
	if cfg.Addr == "" {
		log.Printf("[redis] addr empty, daily digest disabled")
		return nil
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[redis] ping failed (%s): %v — daily digest unavailable", cfg.Addr, err)
		return nil
	}
	log.Printf("[redis] connected: %s db=%d", cfg.Addr, cfg.DB)
	return client
}
