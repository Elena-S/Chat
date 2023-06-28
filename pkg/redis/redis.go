package redis

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/Elena-S/Chat/pkg/srcmng"
	"github.com/redis/go-redis/v9"
)

var Client *redisClient = new(redisClient)

func init() {
	srcmng.SourceKeeper.Add(Client)
}

type redisClient struct {
	client *redis.Client
	once   sync.Once
}

func (rc *redisClient) MustLaunch() {
	rc.once.Do(func() {
		const envRedisUserPwd = "REDIS_USER_PASSWORD"
		pwd := os.Getenv(envRedisUserPwd)
		if pwd == "" {
			log.Fatalf("redis: no password was provided in %s env var", envRedisUserPwd)
		}

		rc.client = redis.NewClient(&redis.Options{
			Addr:       "redis:6379",
			Username:   "chat",
			Password:   pwd,
			DB:         0,
			ClientName: "Chat",
		})
	})
}

func (rc *redisClient) SetEx(ctx context.Context, key string, value any, expiration time.Duration) error {
	result := rc.initClient().SetEx(ctx, key, value, expiration)
	return result.Err()
}

func (rc *redisClient) GetEx(ctx context.Context, key string, expiration time.Duration) (string, error) {
	result := rc.initClient().GetEx(ctx, key, expiration)
	if result.Err() != nil {
		return "", result.Err()
	}
	return result.Val(), nil
}

func (rc *redisClient) Close() error {
	if rc.client == nil {
		return nil
	}
	return rc.client.Close()
}

func (rc *redisClient) initClient() *redis.Client {
	rc.MustLaunch()
	return rc.client
}
