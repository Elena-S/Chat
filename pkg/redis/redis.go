package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/srcmng"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/websocket"
)

var _ PubSubSrcManager = (*redisClient)(nil)
var Client *redisClient = new(redisClient)

type PubSubSrcManager interface {
	broker.PubSub
	srcmng.SourceManager
}

type redisClient struct {
	client             *redis.Client
	once               sync.Once
	minStorageDuration time.Duration
}

func (rc *redisClient) MustLaunch() {
	rc.once.Do(func() {
		const envRedisUserPwd = "REDIS_USER_PASSWORD"
		pwd := os.Getenv(envRedisUserPwd)
		if pwd == "" {
			logger.ChatLogger.WithEventField("redis client creation").Fatal(fmt.Sprintf("redis: no password was provided in %s env var", envRedisUserPwd))
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

func (rc *redisClient) SetMinStorageDuration(d time.Duration) {
	rc.minStorageDuration = d
}

func (rc *redisClient) Subscribe(ctx context.Context, stream string, payload map[any]any) error {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: valueMessage([]byte{}),
		MinID:  rc.minID(),
	}
	return rc.initClient().XAdd(ctx, args).Err()
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
	return rc.initClient().XAdd(ctx, args).Err()
}

func (rc *redisClient) ReadMessages(ctx context.Context, stream string, ws *websocket.Conn, payload map[any]any, ctxLogger logger.Logger) {
	chStopReading, err := broker.RetrieveValue(broker.KeyChStopReading, payload, (chan struct{})(nil))
	if err != nil {
		ctxLogger.Panic(err)
	}
	defer close(chStopReading)

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
			Block:   time.Millisecond * 500,
			Count:   100,
		}
		cmd := rc.initClient().XRead(ctx, args)
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

				err = chats.SendMessage([]byte(value), ws)
				if err != nil {
					if errors.Is(err, net.ErrClosed) {
						break
					}
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

func (rc *redisClient) initClient() *redis.Client {
	rc.MustLaunch()
	return rc.client
}

func (rc *redisClient) minID() string {
	return strconv.FormatInt(time.Now().Add(-rc.minStorageDuration).UnixMilli(), 10) + "-0"
}

func valueMessage(message []byte) map[string]interface{} {
	return map[string]interface{}{"message": message}
}
