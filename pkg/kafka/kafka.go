package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"go.uber.org/fx"
)

const keyConsumer = "kafkaConsumer"

var _ broker.PubSub = (*kafkaClient)(nil)

var Module = fx.Module("kafka",
	fx.Provide(
		func() (*kafkaClient, broker.PubSub, error) {
			kc, err := NewClient()
			return kc, kc, err
		},
		// fx.Annotate(NewClient, fx.As(new(broker.PubSub))),
	),
	fx.Invoke(registerFunc),
)

type kafkaClient struct {
	clientID           string
	producer           *kafka.Producer
	minStorageDuration string
	readingTimeout     time.Duration
	cunsumersNum       uint64
}

func NewClient() (*kafkaClient, error) {
	clientID := uuid.NewString()
	//TODO: need config
	pr, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": "kafka:9092",
		"client.id":         clientID,
	})
	if err != nil {
		return nil, err
	}
	c := &kafkaClient{
		clientID: clientID,
		producer: pr,
	}
	return c, nil
}

func registerFunc(lc fx.Lifecycle, c *kafkaClient) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return c.Close()
		},
	})
}

func (kc *kafkaClient) SetMinStorageDuration(d time.Duration) {
	kc.minStorageDuration = strconv.FormatUint(uint64(d/time.Millisecond), 10)
}

func (kc *kafkaClient) SetReadingTimeout(d time.Duration) {
	kc.readingTimeout = time.Millisecond * d
}

func (kc *kafkaClient) Subscribe(ctx context.Context, topic string, payload map[any]any) error {
	id := fmt.Sprintf("%s-%s", kc.clientID, strconv.FormatUint(atomic.AddUint64(&kc.cunsumersNum, 1), 10))
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  "kafka:9092",
		"client.id":          id,
		"group.id":           id,
		"enable.auto.commit": false,
	})
	if err != nil {
		return err
	}
	payload[keyConsumer] = consumer

	err = consumer.Subscribe(topic, nil)
	if err != nil {
		return err
	}
	kac, err := kafka.NewAdminClientFromProducer(kc.producer)
	if err != nil {
		return err
	}
	res, err := kac.CreateTopics(ctx, []kafka.TopicSpecification{{
		Topic:         topic,
		NumPartitions: 1,
		Config:        map[string]string{"retention.ms": kc.minStorageDuration}}})
	if err != nil {
		return err
	}
	errKafka := res[0].Error
	if errKafka.Code() != kafka.ErrNoError && errKafka.Code() != kafka.ErrTopicAlreadyExists {
		return errKafka
	}
	return nil
}

func (kc *kafkaClient) Unsubscribe(ctx context.Context, topic string, payload map[any]any) (err error) {
	consumer, err := retrieveValue(keyConsumer, payload, (*kafka.Consumer)(nil))
	if err != nil {
		return
	}
	if err = consumer.Unsubscribe(); err != nil {
		return
	}
	if err = consumer.Close(); err != nil {
		return
	}
	kac, err := kafka.NewAdminClientFromProducer(kc.producer)
	if err != nil {
		return
	}
	consumerName := consumer.String()
	groups := []string{consumerName[:strings.IndexRune(consumerName, '#')]}
	res, err := kac.DeleteConsumerGroups(ctx, groups)
	if err != nil {
		return
	}
	errKafka := res.ConsumerGroupResults[0].Error
	if errKafka.Code() != kafka.ErrNoError {
		return errKafka
	}
	return nil
}

func (kc *kafkaClient) Publish(ctx context.Context, topic string, message []byte) error {
	chEvent := make(chan kafka.Event)
	err := kc.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: message,
	}, chEvent)
	if err != nil {
		return err
	}

	select {
	case data := <-chEvent:
		if ev, ok := data.(*kafka.Message); ok {
			if ev.TopicPartition.Error != nil {
				return ev.TopicPartition.Error
			}
		} else {
			return fmt.Errorf("kafka: gotten event data does not match type %T, got type %T", ev, data)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (kc *kafkaClient) ReadMessages(ctx context.Context, topic string, messageHandler func(xmessage []byte) error, payload map[any]any, ctxLogger *logger.Logger) {
	consumer, err := retrieveValue(keyConsumer, payload, (*kafka.Consumer)(nil))
	if err != nil {
		ctxLogger.Panic(err)
	}
	for {
		if err := ctx.Err(); err != nil {
			if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				ctxLogger.Error(err.Error())
			}
			break
		}
		xmessage, err := consumer.ReadMessage(kc.readingTimeout)
		if errKafka, ok := err.(kafka.Error); ok && errKafka.IsTimeout() {
			continue
		} else if err != nil {
			ctxLogger.Error(err.Error())
			break
		}
		err = messageHandler(xmessage.Value)
		if err != nil {
			ctxLogger.Error(err.Error())
			continue
			//TODO: add to queue with errors
		}
		if _, err = consumer.CommitMessage(xmessage); err != nil {
			ctxLogger.Error(err.Error())
		}
	}
}

func (kc *kafkaClient) Close() error {
	kc.producer.Close()
	return nil
}

type valueObjects interface {
	*kafka.Consumer
}

func retrieveValue[V valueObjects](key string, values map[any]any, _ V) (V, error) {
	value, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("kafka: the given parameter \"values\" does not contain key \"%s\"", key)
	}
	object, ok := value.(V)
	if !ok {
		return nil, fmt.Errorf("kafka: type of value gotten by key \"%s\" is %T, expected %T", key, value, object)
	}
	return object, nil
}
