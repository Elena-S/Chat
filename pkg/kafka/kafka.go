package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/chats"
	"github.com/Elena-S/Chat/pkg/logger"
	"github.com/Elena-S/Chat/pkg/srcmng"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"golang.org/x/net/websocket"
)

const keyConsumer = "kafkaConsumer"

var clientName = "chat_0001" //TODO: should be different for every single instance

var _ PubSubSrcManager = (*kafkaClient)(nil)
var Client *kafkaClient = &kafkaClient{
	producer: &producer{},
}

type PubSubSrcManager interface {
	broker.PubSub
	srcmng.SourceManager
}

type kafkaClient struct {
	producer           *producer
	minStorageDuration string
	cunsumersNum       uint64
}

type producer struct {
	instance   *kafka.Producer
	onceLaunch sync.Once
}

func (p *producer) MustLaunch() {
	p.onceLaunch.Do(func() {
		pr, err := kafka.NewProducer(&kafka.ConfigMap{
			"bootstrap.servers": "kafka:9092",
			"client.id":         clientName,
		})
		if err != nil {
			logger.ChatLogger.Fatal(err.Error())
		}
		Client.producer.instance = pr
	})
}

func (p *producer) Close() error {
	if p.instance == nil || p.instance.IsClosed() {
		return nil
	}
	p.instance.Close()
	return nil
}

func (p *producer) initClient() *kafka.Producer {
	p.MustLaunch()
	return p.instance
}

func (kc *kafkaClient) MustLaunch() {
	kc.producer.MustLaunch()
}

func (kc *kafkaClient) SetMinStorageDuration(d time.Duration) {
	kc.minStorageDuration = strconv.FormatUint(uint64(d/time.Millisecond), 10)
}

func (kc *kafkaClient) Subscribe(ctx context.Context, topic string, values map[any]any) error {
	id := fmt.Sprintf("%s_%s", clientName, strconv.FormatUint(atomic.AddUint64(&kc.cunsumersNum, 1), 10))
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  "kafka:9092",
		"client.id":          id,
		"group.id":           id,
		"enable.auto.commit": false,
	})
	if err != nil {
		return err
	}

	values[keyConsumer] = consumer

	err = consumer.SubscribeTopics([]string{topic}, nil)
	if err != nil {
		return err
	}

	kac, err := kafka.NewAdminClientFromConsumer(consumer)
	if err != nil {
		return err
	}
	configs := []kafka.ConfigEntry{{
		Name:      "retention.ms",
		Value:     kc.minStorageDuration,
		Operation: kafka.AlterOperationSet,
	}}
	crr, err := kac.AlterConfigs(ctx, []kafka.ConfigResource{{
		Type:   kafka.ResourceTopic,
		Name:   topic,
		Config: configs,
	}})
	if err != nil {
		return err
	}
	errs := make([]error, len(configs))
	i := 0
	for _, res := range crr {
		errKafka := res.Error
		if errKafka.Code() != kafka.ErrNoError {
			errs[i] = errKafka
			i++
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs[:i]...)
	}

	return nil
}

func (kc *kafkaClient) Unsubscribe(ctx context.Context, topic string, values map[any]any) (err error) {
	consumer, err := retrieveValue(keyConsumer, values, (*kafka.Consumer)(nil))
	if err != nil {
		return err
	}

	defer func() {
		if errClose := consumer.Close(); errClose != nil && err == nil {
			err = errClose
		}
	}()

	if err = consumer.Unsubscribe(); err != nil {
		return err
	}

	kac, err := kafka.NewAdminClientFromConsumer(consumer)
	if err != nil {
		return err
	}

	consumerName := consumer.String()
	groups := []string{consumerName[:strings.IndexRune(consumerName, '#')]}
	option := kafka.SetAdminRequestTimeout(time.Second * 30)
	res, err := kac.DeleteConsumerGroups(ctx, groups, option)
	if err != nil {
		return err
	}
	errKafka := res.ConsumerGroupResults[0].Error
	if errKafka.Code() != kafka.ErrNoError {
		return errKafka
	}
	return nil
}

func (kc *kafkaClient) Publish(ctx context.Context, topic string, message []byte) error {
	chEvent := make(chan kafka.Event)
	err := kc.producer.initClient().Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: message,
	}, chEvent)
	if err != nil {
		return err
	}

	data := <-chEvent
	if ev, ok := data.(*kafka.Message); ok {
		if ev.TopicPartition.Error != nil {
			return ev.TopicPartition.Error
		}
	}
	return nil
}

func (kc *kafkaClient) ReadMessages(ctx context.Context, topic string, ws *websocket.Conn, values map[any]any, ctxLogger logger.Logger) {
	consumer, err := retrieveValue(keyConsumer, values, (*kafka.Consumer)(nil))
	if err != nil {
		ctxLogger.Fatal(err.Error())
	}
	for {
		if err := ctx.Err(); err != nil {
			if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				ctxLogger.Error(err.Error())
			}
			return
		}
		xmessage, err := consumer.ReadMessage(time.Millisecond * 500)
		if errKafka, ok := err.(kafka.Error); ok && errKafka.IsTimeout() {
			continue
		} else if err != nil {
			ctxLogger.Error(err.Error())
			return
		}
		message := new(chats.Message)
		if err = json.Unmarshal(xmessage.Value, message); err != nil {
			ctxLogger.Error(err.Error())
			continue
		}

		if message.Type == chats.MessageTypeTyping && message.Date.Add(time.Second*2).UnixMilli() < time.Now().UnixMilli() {
			if _, err = consumer.CommitMessage(xmessage); err != nil {
				ctxLogger.Error(err.Error())
			}
			continue
		}
		err = websocket.JSON.Send(ws, message)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			ctxLogger.Error(err.Error())
			//TODO: add to queue with errors
			continue
		}
		if _, err = consumer.CommitMessage(xmessage); err != nil {
			ctxLogger.Error(err.Error())
		}
	}
}

func (kc *kafkaClient) Close() error {
	return kc.producer.Close()
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
		return nil, fmt.Errorf("kafka: type of value gotten by key \"%s\" is %T, expected %T", key, value, (V)(nil))
	}
	return object, nil
}
