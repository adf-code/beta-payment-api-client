package repository

import (
	"beta-payment-api-client/internal/dto"
	"beta-payment-api-client/internal/entity"
	pkgKafka "beta-payment-api-client/internal/pkg/kafka"
	"context"
	"database/sql"
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
	SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error
	GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error)
	PublishSuccessEvent(ctx context.Context, id uuid.UUID) error
	FetchPaymentStatus(ctx context.Context, id uuid.UUID) (string, error)
	ReadKafkaMessage(ctx context.Context) (string, error)
	Store(ctx context.Context, tx *sql.Tx, payment *entity.PaymentRecord) error
	FetchByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error)
	FetchByIDRedis(ctx context.Context, id uuid.UUID) (int64, error)
	StoreRedis(ctx context.Context, id uuid.UUID) error
	PersistPollingTask(ctx context.Context, id uuid.UUID) error
	RemovePollingTask(ctx context.Context, id uuid.UUID) error
	RestorePollingTasks(ctx context.Context) ([]uuid.UUID, error)
}

type paymentRecordRepoRedis struct {
	redisClient              *redis.Client
	kafkaProducerClient      *pkgKafka.KafkaProducerClient
	kafkaConsumerClient      *pkgKafka.KafkaConsumerClient
	DB                       *sql.DB
	paymentServerAPIKey      string
	KafkaTopicPaymentSuccess string
}

func NewPaymentRecordRepository(redisClient *redis.Client,
	kafkaProducerClient *pkgKafka.KafkaProducerClient,
	kafkaConsumerClient *pkgKafka.KafkaConsumerClient,
	db *sql.DB,
	paymentServerAPIKey string,
	KafkaTopicPaymentSuccess string) PaymentRecordRepository {
	return &paymentRecordRepoRedis{
		redisClient:              redisClient,
		kafkaProducerClient:      kafkaProducerClient,
		kafkaConsumerClient:      kafkaConsumerClient,
		DB:                       db,
		paymentServerAPIKey:      paymentServerAPIKey,
		KafkaTopicPaymentSuccess: KafkaTopicPaymentSuccess,
	}
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

func (p *paymentRecordRepoRedis) Store(ctx context.Context, tx *sql.Tx, paymentRecord *entity.PaymentRecord) error {
	return tx.QueryRowContext(
		ctx,
		"INSERT INTO payment_records (id, tag, description, amount, status) VALUES ($1, $2, $3, $4, $5) RETURNING created_at, updated_at",
		paymentRecord.ID, paymentRecord.Tag, paymentRecord.Description, paymentRecord.Amount, paymentRecord.Status,
	).Scan(&paymentRecord.CreatedAt, &paymentRecord.UpdatedAt)
}

func (p *paymentRecordRepoRedis) FetchByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error) {
	var paymentRecord entity.PaymentRecord
	err := p.DB.QueryRowContext(ctx, "SELECT id, tag, amount, status, created_at, updated_at FROM payment_records WHERE id = $1 AND deleted_at is null", id).
		Scan(&paymentRecord.ID, &paymentRecord.Tag, &paymentRecord.Amount, &paymentRecord.Status, &paymentRecord.CreatedAt, &paymentRecord.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return &paymentRecord, nil
}

func (p *paymentRecordRepoRedis) FetchByIDRedis(ctx context.Context, id uuid.UUID) (int64, error) {
	redisKey := fmt.Sprintf("kafka:seen:%s", id.String())
	return p.redisClient.Exists(ctx, redisKey).Result()
}

func (p *paymentRecordRepoRedis) StoreRedis(ctx context.Context, id uuid.UUID) error {
	redisKey := fmt.Sprintf("kafka:seen:%s", id.String())
	return p.redisClient.Set(ctx, redisKey, "1", 10*time.Minute).Err()
}

func (p *paymentRecordRepoRedis) PersistPollingTask(ctx context.Context, id uuid.UUID) error {
	return p.redisClient.SAdd(ctx, "polling_tasks", id.String()).Err()
}

func (p *paymentRecordRepoRedis) RemovePollingTask(ctx context.Context, id uuid.UUID) error {
	return p.redisClient.SRem(ctx, "polling_tasks", id.String()).Err()
}

func (p *paymentRecordRepoRedis) RestorePollingTasks(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := p.redisClient.SMembers(ctx, "polling_tasks").Result()
	if err != nil {
		return nil, err
	}

	var result []uuid.UUID
	for _, idStr := range ids {
		id, err := uuid.Parse(idStr)
		if err != nil {
			log.Println("❌ Invalid UUID in Redis:", idStr)
			continue
		}
		result = append(result, id)
	}
	return result, nil
}
