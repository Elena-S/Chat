package redis

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const envRedisUserPwd = "REDIS_USER_PASSWORD"

var (
	client *redisClient
	once   sync.Once
)

func Client() *redisClient {
	once.Do(func() {
		pwd := os.Getenv(envRedisUserPwd)
		if pwd == "" {
			log.Fatalf("redis: no password was provided in %s env var", envRedisUserPwd)
		}

		client = &redisClient{
			client: redis.NewClient(&redis.Options{
				Addr:       "redis:6379",
				Username:   "chat",
				Password:   pwd,
				DB:         0,
				ClientName: "Chat",
			}),
		}
	})

	return client
}

type redisClient struct {
	client *redis.Client
}

func (rc *redisClient) SetEx(ctx context.Context, key string, value any, expiration time.Duration) error {
	result := rc.client.SetEx(ctx, key, value, expiration)
	return result.Err()
}

func (rc *redisClient) GetEx(ctx context.Context, key string, expiration time.Duration) (string, error) {
	result := rc.client.GetEx(ctx, key, expiration)
	if result.Err() != nil {
		return "", result.Err()
	}
	return result.Val(), nil

}

func (rc *redisClient) Close() error {
	return rc.client.Close()
}
