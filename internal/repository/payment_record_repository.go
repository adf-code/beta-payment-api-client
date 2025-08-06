package repository

import (
	"beta-payment-api-client/internal/dto"
	"beta-payment-api-client/internal/entity"
	pkgKafka "beta-payment-api-client/internal/pkg/kafka"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

var tasks sync.Map

type PaymentStatus struct {
	Status string `json:"status"`
}

type PaymentRecordRepository interface {
	Store(ctx context.Context, paymentID string, result entity.PaymentRecord) error
	SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error
	GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error)
	PublishSuccessEvent(ctx context.Context, id uuid.UUID) error
	FetchPaymentStatus(ctx context.Context, id uuid.UUID) (string, error)
	ReadKafkaMessage(ctx context.Context) (string, error)
}

type paymentRecordRepoRedis struct {
	redisClient              *redis.Client
	kafkaProducerClient      *pkgKafka.KafkaProducerClient
	kafkaConsumerClient      *pkgKafka.KafkaConsumerClient
	paymentServerAPIKey      string
	KafkaTopicPaymentSuccess string
}

func NewPaymentRecordRepository(redisClient *redis.Client,
	kafkaProducerClient *pkgKafka.KafkaProducerClient,
	kafkaConsumerClient *pkgKafka.KafkaConsumerClient,
	paymentServerAPIKey string,
	KafkaTopicPaymentSuccess string) PaymentRecordRepository {
	return &paymentRecordRepoRedis{
		redisClient:              redisClient,
		kafkaProducerClient:      kafkaProducerClient,
		kafkaConsumerClient:      kafkaConsumerClient,
		paymentServerAPIKey:      paymentServerAPIKey,
		KafkaTopicPaymentSuccess: KafkaTopicPaymentSuccess,
	}
}

func (p *paymentRecordRepoRedis) Store(ctx context.Context, paymentID string, result entity.PaymentRecord) error {
	key := fmt.Sprintf("payment-check:%s:history", paymentID)
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return p.redisClient.RPush(ctx, key, data).Err()
}

func (p *paymentRecordRepoRedis) SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error {
	key := fmt.Sprintf("retry:%s", id.String())
	value := time.Now().Add(delay).Unix()
	return p.redisClient.Set(ctx, key, value, delay).Err()
}

func (p *paymentRecordRepoRedis) GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error) {
	key := fmt.Sprintf("retry:%s", id.String())
	timestamp, err := p.redisClient.Get(ctx, key).Int64()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(timestamp, 0), nil
}

func (p *paymentRecordRepoRedis) PublishSuccessEvent(ctx context.Context, id uuid.UUID) error {
	msg := kafka.Message{
		Key:   []byte(fmt.Sprintf("%s", p.KafkaTopicPaymentSuccess)),
		Value: []byte(id.String()),
	}
	err := p.kafkaProducerClient.Writer.WriteMessages(ctx, msg)
	if err != nil {
		log.Println("Error publishing Kafka message:", err)
		return err
	}
	log.Println("✅ Kafka message published:", msg.Value)
	return nil
}

func (p *paymentRecordRepoRedis) ReadKafkaMessage(ctx context.Context) (string, error) {
	msg, err := p.kafkaConsumerClient.Reader.ReadMessage(ctx)
	if err != nil {
		return "", err
	}
	return string(msg.Value), nil
}

func (p *paymentRecordRepoRedis) FetchPaymentStatus(ctx context.Context, id uuid.UUID) (string, error) {
	url := fmt.Sprintf("http://localhost:8080/api/v1/payments/%s", id.String())

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Println("❌ Failed to create request:", err)
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.paymentServerAPIKey))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("❌ HTTP request failed:", err)
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("❌ Failed to read body:", err)
		return "", err
	}

	log.Println("📦 Payment API response:", string(body))

	var result dto.GetPaymentByIDResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Println("❌ Failed to unmarshal JSON:", err)
		return "", err
	}

	return result.Data.Status, nil
}
