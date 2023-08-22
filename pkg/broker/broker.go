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

const KeyChStopReading = "brokerKeyChStopReading"
const keyCancelFunc = "brokerCancelFunc"

var ErrBrokerIsNotAssigned error = errors.New("broker: the message broker is not assigned")
var ErrBrokerReassignment error = errors.New("broker: message broker reassignment is not available")

type PubSub interface {
	SetMinStorageDuration(d time.Duration)
	Subscribe(ctx context.Context, topic string, payload map[any]any) error
	Unsubscribe(ctx context.Context, topic string, payload map[any]any) error
	Publish(ctx context.Context, topic string, message []byte) error
	ReadMessages(ctx context.Context, topic string, ws *websocket.Conn, payload map[any]any, ctxLogger logger.Logger)
}

var onceAssign sync.Once
var messageBroker PubSub

func MustAssign(client PubSub) {
	err := ErrBrokerReassignment
	ctxLogger := logger.ChatLogger.WithEventField("broker assignment")
	defer func() {
		if err != nil {
			ctxLogger.Fatal(err.Error())
		}
		ctxLogger.Sync()
	}()
	onceAssign.Do(func() {
		messageBroker = client
		messageBroker.SetMinStorageDuration(time.Hour * 72)
		if err == ErrBrokerReassignment {
			err = nil
		}
	})
}

func Subscribe(ctx context.Context, topic string, ws *websocket.Conn, payload map[any]any) (err error) {
	ctxLogger := logger.ChatLogger.WithEventField("broker subscription").With("topic", topic)
	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		} else if data := recover(); data != nil {
			ctxLogger.Error(fmt.Sprintf("broker: panic raised, %v", data))
		}
		ctxLogger.Sync()
	}()

	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}

	err = messageBroker.Subscribe(ctx, topic, payload)
	if err != nil {
		return err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	payload[keyCancelFunc] = cancelFunc
	payload[KeyChStopReading] = make(chan struct{})

	go func() {
		ctxLogger := logger.ChatLogger.WithEventField("broker reading messages").With("topic", topic)
		defer func() {
			if data := recover(); data != nil {
				ctxLogger.Error(fmt.Sprintf("broker: panic raised, %v", data))
			}
			ctxLogger.Sync()
		}()
		messageBroker.ReadMessages(ctx, topic, ws, payload, ctxLogger)
	}()

	return nil
}

func Unsubscribe(ctx context.Context, topic string, payload map[any]any) (err error) {
	ctxLogger := logger.ChatLogger.WithEventField("broker unsubscription").With("topic", topic)
	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		} else if data := recover(); data != nil {
			ctxLogger.Error(fmt.Sprintf("broker: panic raised, %v", data))
		}
		ctxLogger.Sync()
	}()
	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}
	cancelFunc, err := RetrieveValue(keyCancelFunc, payload, context.CancelFunc(nil))
	if err != nil {
		return err
	}
	cancelFunc()

	chStopReading, err := RetrieveValue(KeyChStopReading, payload, (chan struct{})(nil))
	if err != nil {
		return err
	}
	<-chStopReading

	return messageBroker.Unsubscribe(ctx, topic, payload)
}

func Publish(ctx context.Context, topic string, message []byte) (err error) {
	ctxLogger := logger.ChatLogger.WithEventField("broker publish message").With("topic", topic).With("message", message)
	defer func() {
		if err != nil {
			ctxLogger.Error(err.Error())
		} else if data := recover(); data != nil {
			ctxLogger.Error(fmt.Sprintf("broker: panic raised, %v", data))
		}
		ctxLogger.Sync()
	}()
	if messageBroker == nil {
		return ErrBrokerIsNotAssigned
	}
	return messageBroker.Publish(ctx, topic, message)
}

type valueObjects interface {
	context.CancelFunc | chan struct{}
}

func RetrieveValue[V valueObjects](key string, values map[any]any, _ V) (V, error) {
	value, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("broker: the given parameter \"values\" does not contain key \"%s\"", key)
	}
	object, ok := value.(V)
	if !ok {
		return nil, fmt.Errorf("broker: type of value gotten by key \"%s\" is %T, expected %T", key, value, object)
	}
	return object, nil
}
