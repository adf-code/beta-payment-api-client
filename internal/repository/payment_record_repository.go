package repository

import (
	"beta-payment-api-client/internal/entity"
	pkgKafka "beta-payment-api-client/internal/pkg/kafka"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
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
	Store(paymentID string, result entity.PaymentRecord) error
	SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error
	GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error)
	StartPolling(ctx context.Context, id uuid.UUID) error
}

type paymentRecordRepoRedis struct {
	redisClient         *redis.Client
	kafkaProducerClient *pkgKafka.KafkaProducerClient
	kafkaConsumerClient *pkgKafka.KafkaConsumerClient
}

func NewPaymentRecordRepository(redisClient *redis.Client, kafkaProducerClient *pkgKafka.KafkaProducerClient, kafkaConsumerClient *pkgKafka.KafkaConsumerClient) PaymentRecordRepository {
	return &paymentRecordRepoRedis{
		redisClient:         redisClient,
		kafkaProducerClient: kafkaProducerClient,
		kafkaConsumerClient: kafkaConsumerClient,
	}
}

func (r *paymentRecordRepoRedis) Store(paymentID string, result entity.PaymentRecord) error {
	ctx := context.Background()
	key := fmt.Sprintf("payment-check:%s:history", paymentID)

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return r.redisClient.RPush(ctx, key, data).Err()
}

func (r *paymentRecordRepoRedis) SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error {
	return r.redisClient.Set(ctx, fmt.Sprintf("retry:%s", id), time.Now().Add(delay).Unix(), delay).Err()
}

func (r *paymentRecordRepoRedis) GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error) {
	timestamp, err := r.redisClient.Get(ctx, fmt.Sprintf("retry:%s", id)).Int64()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(timestamp, 0), nil
}

func (p *paymentRecordRepoRedis) PublishSuccessEvent(id uuid.UUID) error {
	err := p.kafkaProducerClient.Writer.WriteMessages(nil, kafka.Message{
		Key:   []byte("payment_success"),
		Value: []byte(fmt.Sprintf("%s", id)),
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *paymentRecordRepoRedis) StartPolling(ctx context.Context, id uuid.UUID) error {
	go func() {
		delay := 10 * time.Second
		maxDelay := 60 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
				log.Println("Checking:", id)

				status, err := fetchStatus(id)
				if err != nil {
					log.Println("Fetch error:", err)
				}

				if status == "PAID" || status == "UNPAID" {
					log.Printf("Payment %s finalized: %s", id, status)
					_ = p.PublishSuccessEvent(id)
					return
				}

				_ = p.SetNextRetry(ctx, id, delay)
				time.Sleep(delay)

				if delay < maxDelay {
					delay *= 2
				}
			}
		}
	}()
	tasks.Store(id, true)
	return nil
}

func fetchStatus(id uuid.UUID) (string, error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:8000/payment/check/%s", id))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result PaymentStatus
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result.Status, err
}

func (p *paymentRecordRepoRedis) BoostOtherTasks(id uuid.UUID) error {
	tasks.Range(func(key, _ interface{}) bool {
		paymentID := key.(uuid.UUID)
		if paymentID != id {
			go p.StartPolling(context.Background(), paymentID)
		}
		return true
	})
	return nil
}

func (p *paymentRecordRepoRedis) StartConsumer(kafkaConsumerClient *pkgKafka.KafkaConsumerClient) error {
	go func() {
		for {
			msg, err := kafkaConsumerClient.Reader.ReadMessage(context.Background())
			if err != nil {
				log.Println("kafka read error:", err)
				continue
			}

			// Parse paymentID from Kafka message
			paymentIDStr := string(msg.Value)
			paymentID, err := uuid.Parse(paymentIDStr)
			if err != nil {
				log.Println("invalid UUID from Kafka message:", paymentIDStr, "error:", err)
				continue
			}

			log.Println("Boost triggered by:", paymentID)
			err = p.BoostOtherTasks(paymentID)
			if err != nil {
				log.Println("BoostOtherTasks error:", err)
			}
		}
	}()
	return nil
}
