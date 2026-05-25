package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// Set 保存每日摘要到 Redis
func (r *DigestRepository) Set(digest *DailyDigest) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("redis client not available")
	}
	if digest == nil || digest.Date == "" || digest.Text == "" {
		return fmt.Errorf("invalid digest: date and text are required")
	}

	ctx := context.Background()
	data, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("marshal digest: %w", err)
	}

	// 保存到日期特定的 key
	dateKey := digestKey(digest.Date)
	if err := r.client.Set(ctx, dateKey, data, 7*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", dateKey, err)
	}

	// 同时更新 latest key
	if err := r.client.Set(ctx, digestLatestKey, data, 7*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("redis set latest: %w", err)
	}

	return nil
}
