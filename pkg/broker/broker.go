package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	"golang.org/x/net/websocket"
)

const keyCancelFunc = "brokerCancelFunc"

var ErrBrokerIsNotAssigned error = errors.New("broker: the message broker is not assigned")

type PubSub interface {
	SetMinStorageDuration(d time.Duration)
	Subscribe(ctx context.Context, topic string, values map[any]any) error
	Unsubscribe(ctx context.Context, topic string, values map[any]any) error
	Publish(ctx context.Context, topic string, message []byte) error
	ReadMessages(ctx context.Context, topic string, ws *websocket.Conn, values map[any]any, ctxLogger logger.Logger)
}

var onceAssign sync.Once
var messageBroker PubSub

func MustAssign(client PubSub) {
	err := errors.New("broker: message broker reassignment is not available")
	onceAssign.Do(func() {
		messageBroker = client
		messageBroker.SetMinStorageDuration(time.Hour * 72)
		err = nil
	})
	if err != nil {
		logger.ChatLogger.WithEventField("broker assignment").Fatal(err.Error())
	}
}

func Subscribe(ctx context.Context, topic string, ws *websocket.Conn, values map[any]any) error {
	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}

	if err := messageBroker.Subscribe(ctx, topic, values); err != nil {
		return err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	values[keyCancelFunc] = cancelFunc
	ctxLogger := logger.ChatLogger.WithEventField("reading messages from broker").With("topic", topic)

	go messageBroker.ReadMessages(ctx, topic, ws, values, ctxLogger)

	return nil
}

func Unsubscribe(ctx context.Context, topic string, values map[any]any) error {
	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}
	cancelFunc, err := retrieveValue(keyCancelFunc, values, context.CancelFunc(nil))
	if err != nil {
		return err
	}
	cancelFunc()
	return messageBroker.Unsubscribe(ctx, topic, values)
}

func Publish(ctx context.Context, topic string, message []byte) error {
	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}
	return messageBroker.Publish(ctx, topic, message)
}

type valueObjects interface {
	context.CancelFunc
}

func retrieveValue[V valueObjects](key string, values map[any]any, _ V) (V, error) {
	value, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("broker: the given parameter \"values\" does not contain key \"%s\"", key)
	}
	object, ok := value.(V)
	if !ok {
		return nil, fmt.Errorf("broker: type of value gotten by key \"%s\" is %T, expected %T", key, value, (V)(nil))
	}
	return object, nil
}
