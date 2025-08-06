package usecase

import (
	"beta-payment-api-client/internal/repository"
	"context"
	"github.com/google/uuid"
	"log"
	"sync"
	"time"
)

type PaymentRecordUseCase interface {
	StartPolling(ctx context.Context, id uuid.UUID) error
	StartConsumer(ctx context.Context) error
	BoostOtherTasks(id uuid.UUID) error
}

type paymentRecordUseCase struct {
	repo  repository.PaymentRecordRepository
	tasks sync.Map
}

func NewPaymentRecordUseCase(repo repository.PaymentRecordRepository) PaymentRecordUseCase {
	return &paymentRecordUseCase{repo: repo}
}

func (u *paymentRecordUseCase) StartPolling(ctx context.Context, id uuid.UUID) error {
	go func() {
		delay := 10 * time.Second
		maxDelay := 60 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
				log.Println("Polling:", id)
				status, err := u.repo.FetchPaymentStatus(ctx, id)
				if err != nil {
					log.Println("Fetch error:", err)
				}

				if status == "PAID" || status == "UNPAID" {
					log.Printf("Finalized: %s -> %s", id, status)
					_ = u.repo.PublishSuccessEvent(ctx, id)
					return
				}

				_ = u.repo.SetNextRetry(ctx, id, delay)
				time.Sleep(delay)

				if delay < maxDelay {
					delay *= 2
				}
			}
		}
	}()
	u.tasks.Store(id, true)
	return nil
}

func (u *paymentRecordUseCase) BoostOtherTasks(id uuid.UUID) error {
	u.tasks.Range(func(key, _ interface{}) bool {
		paymentID := key.(uuid.UUID)
		if paymentID != id {
			go u.StartPolling(context.Background(), paymentID)
		}
		return true
	})
	return nil
}

func (u *paymentRecordUseCase) StartConsumer(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Kafka consumer stopped")
				return
			default:
				paymentIDStr, err := u.repo.ReadKafkaMessage(ctx)
				if err != nil {
					log.Println("Kafka error:", err)
					continue
				}
				paymentID, err := uuid.Parse(paymentIDStr)
				if err != nil {
					log.Println("Invalid UUID:", paymentIDStr)
					continue
				}
				log.Println("Boost triggered by:", paymentID)
				_ = u.BoostOtherTasks(paymentID)
			}
		}
	}()
	return nil
}
