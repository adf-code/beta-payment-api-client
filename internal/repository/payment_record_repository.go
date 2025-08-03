package repository

import (
	"beta-payment-api-client/internal/entity"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

type PaymentRecordRepository interface {
	Store(paymentID string, result entity.PaymentRecord) error
	SetNextRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error
	GetNextRetry(ctx context.Context, id uuid.UUID) (time.Time, error)
}

type paymentRecordRepoRedis struct {
	redisClient *redis.Client
}

func NewPaymentRecordRepository(redisClient *redis.Client) PaymentRecordRepository {
	return &paymentRecordRepoRedis{redisClient: redisClient}
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
