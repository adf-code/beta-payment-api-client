package usecase

import (
	"beta-payment-api-client/internal/entity"
	"beta-payment-api-client/internal/repository"
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"log"
	"sync"
	"time"
)

type PaymentRecordUseCase interface {
	StartPolling(ctx context.Context, id uuid.UUID) error
	StartConsumer(ctx context.Context) error
	BoostOtherTasks(id uuid.UUID) error
	Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error)
}

type paymentRecordUseCase struct {
	paymentRecordRepo repository.PaymentRecordRepository
	tasks             sync.Map
	db                *sql.DB
	logger            zerolog.Logger
}

func NewPaymentRecordUseCase(paymentRecordRepo repository.PaymentRecordRepository, db *sql.DB, logger zerolog.Logger) PaymentRecordUseCase {
	return &paymentRecordUseCase{
		paymentRecordRepo: paymentRecordRepo,
		db:                db,
		logger:            logger,
	}
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
				status, err := u.paymentRecordRepo.FetchPaymentStatus(ctx, id)
				if err != nil {
					log.Println("Fetch error:", err)
				}

				if status == "PAID" || status == "UNPAID" {
					log.Printf("Finalized: %s -> %s", id, status)
					_ = u.paymentRecordRepo.PublishSuccessEvent(ctx, id)
					return
				}

				_ = u.paymentRecordRepo.SetNextRetry(ctx, id, delay)
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
				paymentIDStr, err := u.paymentRecordRepo.ReadKafkaMessage(ctx)
				if err != nil {
					log.Println("Kafka error:", err)
					continue
				}

				paymentID, err := uuid.Parse(paymentIDStr)
				if err != nil {
					log.Println("Invalid UUID:", paymentIDStr)
					continue
				}

				// Deduplication check using Redis
				exists, err := u.paymentRecordRepo.FetchByIDRedis(ctx, paymentID)
				if err != nil {
					log.Println("Redis error:", err)
					continue
				}

				if exists > 0 {
					log.Println("🔁 Kafka message already processed, skipping:", paymentID)
					continue
				}

				// Set the key in Redis to mark as seen
				err = u.paymentRecordRepo.StoreRedis(ctx, paymentID)
				if err != nil {
					log.Println("Failed to set dedup key in Redis:", err)
					continue
				}

				log.Println("⚡ Boost triggered by:", paymentID)
				_ = u.BoostOtherTasks(paymentID)
			}
		}
	}()
	return nil
}

func (uc *paymentRecordUseCase) Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error) {
	uc.logger.Info().Str("usecase", "Create").Msg("⚙️ Store payment records")
	tx, err := uc.db.Begin()
	if err != nil {
		uc.logger.Error().Err(err).Msg("❌ Failed to begin transaction")
		return nil, err
	}

	err = uc.paymentRecordRepo.Store(ctx, tx, &paymentRecord)
	if err != nil {
		tx.Rollback()
		uc.logger.Error().Err(err).Msg("❌ Failed to store payment records, rolling back")
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		uc.logger.Error().Err(err).Msg("❌ Failed to commit transaction")
		return nil, err
	}

	uc.logger.Info().Str("payment_id", paymentRecord.ID.String()).Msg("✅ Payment records created")
	return &paymentRecord, nil
}

func (uc *paymentRecordUseCase) GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error) {
	uc.logger.Info().Str("usecase", "GetByID").Msg("⚙️ Fetching payment records by ID")
	return uc.paymentRecordRepo.FetchByID(ctx, id)
}
