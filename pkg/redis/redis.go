package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Elena-S/Chat/pkg/auth"
	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

var _ broker.PubSub = (*redisClient)(nil)

var Module = fx.Module("redis",
	fx.Provide(
		func() (*redisClient, auth.SetGetterEx) {
			rc := NewClient()
			return rc, rc
		},
		// fx.Annotate(NewClient, fx.As(new(auth.SetGetterEx))),
	),
	fx.Invoke(registerFunc),
)

type redisClient struct {
	client             *redis.Client
	minStorageDuration time.Duration
	readingTimeout     time.Duration
}

func NewClient() *redisClient {
	rc := &redisClient{}
	//TODO: need config
	rc.client = redis.NewClient(&redis.Options{
		Addr:       "redis:6379",
		Username:   "chat",
		Password:   os.Getenv("REDIS_USER_PASSWORD"),
		DB:         0,
		ClientName: "Chat",
	})
	return rc
}

func registerFunc(lc fx.Lifecycle, rc *redisClient) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			status := rc.client.Ping(ctx)
			return status.Err()
		},
		OnStop: func(ctx context.Context) error {
			return rc.Close()
		},
	})
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

func (rc *redisClient) SetMinStorageDuration(d time.Duration) {
	rc.minStorageDuration = d
}

func (rc *redisClient) SetReadingTimeout(d time.Duration) {
	rc.readingTimeout = time.Millisecond * d
}

func (rc *redisClient) Subscribe(ctx context.Context, stream string, payload map[any]any) error {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: valueMessage([]byte{}),
		MinID:  rc.minID(),
	}
	return rc.client.XAdd(ctx, args).Err()
}

func (rc *redisClient) Unsubscribe(ctx context.Context, stream string, payload map[any]any) error {
	return nil
}

func (rc *redisClient) Publish(ctx context.Context, stream string, message []byte) error {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: valueMessage(message),
		MinID:  rc.minID(),
	}
	return rc.client.XAdd(ctx, args).Err()
}

func (rc *redisClient) ReadMessages(ctx context.Context, stream string, messageHandler func(xmessage []byte) error, payload map[any]any, ctxLogger *logger.Logger) {
	offset := "$"
	for {
		if err := ctx.Err(); err != nil {
			if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				ctxLogger.Error(err.Error())
			}
			break
		}
		args := &redis.XReadArgs{
			Streams: []string{stream, offset},
			Block:   rc.readingTimeout,
			Count:   100,
		}
		cmd := rc.client.XRead(ctx, args)
		if err := cmd.Err(); err == redis.Nil {
			continue
		} else if err != nil {
			ctxLogger.Error(err.Error())
			break
		}
		xstreams, err := cmd.Result()
		if err != nil {
			ctxLogger.Error(err.Error())
			break
		}
		for _, xstream := range xstreams {
			for _, xmessage := range xstream.Messages {
				offset = xmessage.ID

				data, ok := xmessage.Values["message"]
				if !ok {
					err = fmt.Errorf("redis: a message with an id %s have no a key \"message\"", xmessage.ID)
					ctxLogger.Error(err.Error())
					continue
				}
				value, ok := data.(string)
				if !ok {
					err = fmt.Errorf("redis: a message with an id %s have data type %T instead of string", xmessage.ID, data)
					ctxLogger.Error(err.Error())
					continue
				}

				if value == "" {
					if err = rc.client.XDel(ctx, stream, xmessage.ID).Err(); err != nil {
						ctxLogger.Error(err.Error())
					}
					continue
				}

				err = messageHandler([]byte(value))
				if err != nil {
					ctxLogger.Error(err.Error())
					//TODO: add to queue with errors
				}
			}
		}
	}
}

func (rc *redisClient) Close() error {
	if rc.client == nil {
		return nil
	}
	return rc.client.Close()
}

func (rc *redisClient) minID() string {
	return strconv.FormatInt(time.Now().Add(-rc.minStorageDuration).UnixMilli(), 10) + "-0"
}

func valueMessage(message []byte) map[string]interface{} {
	return map[string]interface{}{"message": message}
}
