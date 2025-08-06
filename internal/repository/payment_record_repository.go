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
	Store(paymentID string, result entity.PaymentRecord) error
	SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error
	GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error)
	StartPolling(ctx context.Context, id uuid.UUID) error
	StartConsumer(ctx context.Context) error
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
	log.Println("⁉️ nilai topics", p.KafkaTopicPaymentSuccess)
	msg := kafka.Message{
		Key:   []byte(fmt.Sprintf("%s", p.KafkaTopicPaymentSuccess)),
		Value: []byte(id.String()),
	}
	err := p.kafkaProducerClient.Writer.WriteMessages(context.Background(), msg)
	if err != nil {
		log.Println("Error publishing:", err)
		return err
	}
	log.Println("Successfully published message:", msg.Value)
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

				status, err := fetchStatus(id, p.paymentServerAPIKey)
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

func fetchStatus(id uuid.UUID, paymentServerAPIKey string) (string, error) {
	url := fmt.Sprintf("http://localhost:8080/api/v1/payments/%s", id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println("request creation error:", err)
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", paymentServerAPIKey))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("error reading body:", err)
		return "", err
	}

	log.Println("RAW JSON:", string(body))

	var result dto.GetPaymentByIDResponse
	err = json.Unmarshal(body, &result)

	return result.Data.Status, err
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

func (p *paymentRecordRepoRedis) StartConsumer(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Kafka consumer stopped by context")
				return
			default:
				msg, err := p.kafkaConsumerClient.Reader.ReadMessage(ctx) // pass ctx
				if err != nil {
					// If context was canceled, exit
					if ctx.Err() != nil {
						log.Println("Kafka consumer exiting due to context:", ctx.Err())
						return
					}
					log.Println("Kafka read error:", err)
					continue
				}

				// Parse payment ID
				paymentIDStr := string(msg.Value)
				paymentID, err := uuid.Parse(paymentIDStr)
				if err != nil {
					log.Println("Invalid UUID from Kafka message:", paymentIDStr, "error:", err)
					continue
				}

				log.Println("Boost triggered by:", paymentID)
				if err := p.BoostOtherTasks(paymentID); err != nil {
					log.Println("BoostOtherTasks error:", err)
				}
			}
		}
	}()
	return nil
}
