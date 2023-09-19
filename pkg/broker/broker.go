package broker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Elena-S/Chat/pkg/logger"
	"go.uber.org/fx"
)

const KeyChStopReading = "brokerKeyChStopReading"
const keyCancelFunc = "brokerCancelFunc"

type PubSub interface {
	SetMinStorageDuration(d time.Duration)
	SetReadingTimeout(d time.Duration)
	Subscribe(ctx context.Context, topic string, payload map[any]any) error
	Unsubscribe(ctx context.Context, topic string, payload map[any]any) error
	Publish(ctx context.Context, topic string, message []byte) error
	ReadMessages(ctx context.Context, topic string, messageHandler func(xmessage []byte) error, payload map[any]any, ctxLogger *logger.Logger)
	io.Closer
}

var Module = fx.Module("broker",
	fx.Provide(
		NewClient,
	),
)

type BrokerClient struct {
	client PubSub
	logger *logger.Logger
}

type ClientParams struct {
	fx.In
	Client PubSub
	Logger *logger.Logger
}

func NewClient(p ClientParams) *BrokerClient {
	b := &BrokerClient{
		client: p.Client,
		logger: p.Logger,
	}
	//TODO: need config
	b.client.SetMinStorageDuration(time.Hour * 72)
	b.client.SetReadingTimeout(500)
	return b
}

func (b *BrokerClient) Subscribe(ctx context.Context, topic string, messageHandler func(xmessage []byte) error, payload map[any]any) (err error) {
	ctxSub, cancelFuncSub := context.WithTimeout(ctx, time.Second*30)
	defer cancelFuncSub()

	err = b.client.Subscribe(ctxSub, topic, payload)
	if err != nil {
		return err
	}

	newCtx, cancelFunc := context.WithCancel(ctx)
	payload[keyCancelFunc] = cancelFunc
	chStopReading := make(chan struct{})
	payload[KeyChStopReading] = chStopReading

	go func() {
		ctxLogger := b.logger.WithEventField("broker reading messages").With("topic", topic)
		ctxLogger.Info("start")
		defer func() {
			data := recover()
			ctxLogger.OnDefer("broker", err, data, "finish")
		}()
		defer close(chStopReading)
		b.client.ReadMessages(newCtx, topic, messageHandler, payload, ctxLogger)
	}()

	return nil
}

func (b *BrokerClient) Unsubscribe(ctx context.Context, topic string, payload map[any]any) (err error) {
	ctxLogger := b.logger.WithEventField("broker unsubscription")
	ctxLogger.Info("start")
	defer ctxLogger.Info("finish")

	ctxUnsub, cancelFuncUnsub := context.WithTimeout(ctx, time.Second*30)
	defer cancelFuncUnsub()

	cancelFunc, err := retrieveValue(keyCancelFunc, payload, context.CancelFunc(nil))
	if err != nil {
		return err
	}
	cancelFunc()

	chStopReading, err := retrieveValue(KeyChStopReading, payload, (chan struct{})(nil))
	if err != nil {
		return err
	}
	<-chStopReading

	return b.client.Unsubscribe(ctxUnsub, topic, payload)
}

func (b *BrokerClient) Publish(ctx context.Context, topic string, message []byte) (err error) {
	ctxPub, cancelFuncPub := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFuncPub()
	return b.client.Publish(ctxPub, topic, message)
}

type valueObjects interface {
	context.CancelFunc | chan struct{}
}

func retrieveValue[V valueObjects](key string, values map[any]any, _ V) (V, error) {
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
