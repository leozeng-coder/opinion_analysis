package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	digestKeyPrefix = "opinion:daily_digest:"
	digestLatestKey = "opinion:daily_digest:latest"
)

// DailyDigest 仪表盘「今日 AI 摘要」（存 Redis）
type DailyDigest struct {
	Date     string   `json:"date"`
	Text     string   `json:"text"`
	Keywords []string `json:"keywords"`
}

type DigestRepository struct {
	client *redis.Client
}

func NewDigestRepository(client *redis.Client) *DigestRepository {
	if client == nil {
		return nil
	}
	return &DigestRepository{client: client}
}

func digestKey(date string) string {
	return digestKeyPrefix + date
}

func (r *DigestRepository) Get(preferredDate string) (*DailyDigest, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}
	ctx := context.Background()
	if preferredDate != "" {
		if d, err := r.getByKey(ctx, digestKey(preferredDate)); err != nil {
			return nil, err
		} else if d != nil && d.Text != "" {
			return d, nil
		}
	}
	return r.getByKey(ctx, digestLatestKey)
}

func (r *DigestRepository) getByKey(ctx context.Context, key string) (*DailyDigest, error) {
	raw, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %s: %w", key, err)
	}
	var d DailyDigest
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, fmt.Errorf("decode digest: %w", err)
	}
	if d.Text == "" {
		return nil, nil
	}
	return &d, nil
}
