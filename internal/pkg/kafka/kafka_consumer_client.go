package kafka

import (
	"beta-payment-api-client/config"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
	"log"
	"os"
	"time"
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
		Brokers: []string{fmt.Sprintf("%s:%s", k.KafkaHost, k.KafkaPort)},
		Topic:   k.KafkaTopicPaymentSuccess,
		GroupID: "payment-checker-group",

		// ⏱️ Lebih toleran terhadap broker yang baru ready
		MinBytes: 1,               // Minimum fetch size (1B)
		MaxBytes: 10e6,            // Maximum fetch size (10MB)
		MaxWait:  3 * time.Second, // Max time to wait for message

		// 🔁 Auto commit bisa diatur manual jika perlu kontrol penuh
		CommitInterval: time.Second, // default 1s, auto commit offset

		// 🧠 Untuk debug startup
		StartOffset: kafka.FirstOffset, // bisa diganti ke LastOffset sesuai use case
		Logger:      log.New(os.Stdout, "kafka reader: ", 0),
	})
	k.Reader = reader
	return k
}
