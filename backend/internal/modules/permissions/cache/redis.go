package cache

import (
	"context"
	"encoding/json"
	"time"

	"file-workshop/backend/internal/modules/permissions/domain"

	"github.com/redis/go-redis/v9"
)

const decisionTTL = 30 * time.Second

type RedisDecisionCache struct{ client *redis.Client }

func NewRedisDecisionCache(client *redis.Client) *RedisDecisionCache {
	return &RedisDecisionCache{client: client}
}

func (c *RedisDecisionCache) Get(ctx context.Context, key string) (domain.PermissionEvaluation, bool) {
	encoded, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return domain.PermissionEvaluation{}, false
	}
	var value domain.PermissionEvaluation
	if err = json.Unmarshal(encoded, &value); err != nil {
		return domain.PermissionEvaluation{}, false
	}
	return value, true
}

func (c *RedisDecisionCache) Set(ctx context.Context, key string, value domain.PermissionEvaluation) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = c.client.Set(ctx, key, encoded, decisionTTL).Err()
}
