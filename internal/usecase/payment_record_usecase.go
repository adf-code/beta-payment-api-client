package usecase

import (
	"beta-payment-api-client/internal/entity"
	"beta-payment-api-client/internal/repository"
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

var seen sync.Map

type PaymentRecordUseCase interface {
	StartPolling(ctx context.Context, id uuid.UUID) error
	StartConsumer(ctx context.Context) error
	BoostOtherTasks(id uuid.UUID) error
	Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error)
	ListRunningTasks() []uuid.UUID
	RestorePollingTasks(ctx context.Context) error
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
				u.logger.Info().Msgf("⚓️ Polling Payment Record with id: %s", id)
				status, err := u.paymentRecordRepo.FetchPaymentStatus(ctx, id)
				if err != nil {
					u.logger.Error().Msgf("❌ Error fetching Payment Recoed with: %s", err)
				}

				if status == "PAID" || status == "UNPAID" {
					u.logger.Info().Msgf("️🔄 Finalized: %s -> %s", id, status)
					_ = u.paymentRecordRepo.PublishSuccessEvent(ctx, id)

					// ✅ Remove from in-memory tasks
					u.tasks.Delete(id)

					// ✅ Remove from Redis persistence
					_ = u.paymentRecordRepo.RemovePollingTask(ctx, id)
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
	_ = u.paymentRecordRepo.PersistPollingTask(ctx, id)
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
				u.logger.Info().Msgf("⁉️Kafka consumer stopped")
				return
			default:
				paymentIDStr, err := u.paymentRecordRepo.ReadKafkaMessage(ctx)
				if err != nil {
					u.logger.Error().Msgf("❌ Kafka error: %s", err)
					continue
				}
				paymentID, err := uuid.Parse(paymentIDStr)
				if err != nil {
					u.logger.Error().Msgf("❌ Invalid UUID: %s", err)
					continue
				}

				// Deduplication logic
				if _, loaded := seen.LoadOrStore(paymentID.String(), true); loaded {
					u.logger.Info().Msgf("⁉️ Duplicate message ignored: %s", paymentID)
					continue
				}

				u.logger.Info().Msgf("⚡ Boost triggered by: %s", paymentID)
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

func (u *paymentRecordUseCase) ListRunningTasks() []uuid.UUID {
	var ids []uuid.UUID
	u.tasks.Range(func(key, value any) bool {
		if id, ok := key.(uuid.UUID); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func (u *paymentRecordUseCase) RestorePollingTasks(ctx context.Context) error {
	ids, err := u.paymentRecordRepo.RestorePollingTasks(ctx)
	if err != nil {
		u.logger.Error().Err(err).Msg("❌ Failed to restore polling tasks from Redis")
		return err
	}

	for _, id := range ids {
		u.logger.Info().Msgf("♻️ Restoring polling task: %s", id)
		_ = u.StartPolling(ctx, id)
	}
	return nil
}
