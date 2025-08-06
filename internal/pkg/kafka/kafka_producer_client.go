package kafka

import (
	"beta-payment-api-client/config"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

type KafkaProducerClient struct {
	kafkaHost                string
	kafkaPort                string
	kafkaTopicPaymentSuccess string
	Writer                   *kafka.Writer
	logger                   zerolog.Logger
}

func NewKafkaProducerClient(cfg *config.AppConfig, logger zerolog.Logger) *KafkaProducerClient {
	return &KafkaProducerClient{
		kafkaHost:                cfg.KafkaHost,
		kafkaPort:                cfg.KafkaPort,
		kafkaTopicPaymentSuccess: cfg.KafkaTopicPaymentSuccess,
		logger:                   logger,
	}
}

func (k *KafkaProducerClient) InitKafkaProducer() *KafkaProducerClient {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(fmt.Sprintf("%s:%s", k.kafkaHost, k.kafkaPort)),
		Topic:        k.kafkaTopicPaymentSuccess,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
	}
	k.Writer = writer
	return k
}
