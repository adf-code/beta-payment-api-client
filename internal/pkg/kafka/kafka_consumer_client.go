package kafka

import (
	"beta-payment-api-client/config"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

type KafkaConClient interface {
	InitKafkaConsumer() *KafkaConsumerClient
}
type KafkaConsumerClient struct {
	KafkaHost                string
	KafkaPort                string
	KafkaTopicPaymentSuccess string
	Reader                   *kafka.Reader
	logger                   zerolog.Logger
}

func NewKafkaConsumerClient(cfg *config.AppConfig, logger zerolog.Logger) *KafkaConsumerClient {
	return &KafkaConsumerClient{
		KafkaHost:                cfg.KafkaHost,
		KafkaPort:                cfg.KafkaPort,
		KafkaTopicPaymentSuccess: cfg.KafkaTopicPaymentSuccess,
		logger:                   logger,
	}
}

func (k *KafkaConsumerClient) InitKafkaConsumer() *KafkaConsumerClient {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{fmt.Sprintf("%s:%s", k.KafkaHost, k.KafkaHost)},
		Topic:   k.KafkaTopicPaymentSuccess,
		GroupID: "payment-checker-group",
	})
	k.Reader = reader
	return k
}
