package repository

import (
	"beta-payment-api-client/internal/entity"
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
)

type PaymentRecordRepository interface {
	Store(paymentID string, result entity.PaymentRecord) error
}

type paymentRecordRepoRedis struct {
	client *redis.Client
}

func NewPaymentRecordRepository(client *redis.Client) PaymentRecordRepository {
	return &paymentRecordRepoRedis{client: client}
}

func (r *paymentRecordRepoRedis) Store(paymentID string, result entity.PaymentRecord) error {
	ctx := context.Background()
	key := fmt.Sprintf("payment-check:%s:history", paymentID)

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return r.client.RPush(ctx, key, data).Err()
}
