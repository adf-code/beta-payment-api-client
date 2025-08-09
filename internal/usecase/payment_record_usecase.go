package usecase

import (
	"beta-payment-api-client/internal/contextkeys"
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

type taskHandle struct {
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{} // sinyal boost/reset delay
}

type paymentRecordUseCase struct {
	paymentRecordRepo         repository.PaymentRecordRepository
	paymentRecordCheckLogRepo repository.PaymentRecordCheckLogRepository
	tasks                     sync.Map
	db                        *sql.DB
	logger                    zerolog.Logger
}

func NewPaymentRecordUseCase(
	paymentRecordRepo repository.PaymentRecordRepository,
	paymentRecordCheckLogRepo repository.PaymentRecordCheckLogRepository,
	db *sql.DB,
	logger zerolog.Logger) PaymentRecordUseCase {
	return &paymentRecordUseCase{
		paymentRecordRepo:         paymentRecordRepo,
		paymentRecordCheckLogRepo: paymentRecordCheckLogRepo,
		db:                        db,
		logger:                    logger,
	}
}

func (paymentRecordUC *paymentRecordUseCase) StartPolling(ctx context.Context, id uuid.UUID) error {
	go func() {
		delay := 10 * time.Second
		maxDelay := 60 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
				paymentRecordUC.logger.Info().Msgf("⚓️ Polling Payment Record with id: %s", id)
				status, paymentRecordCheckHTTP, err := paymentRecordUC.paymentRecordRepo.FetchPaymentStatus(
					context.WithValue(ctx, contextkeys.CtxKeyPollingDelay, delay),
					id,
				)

				err = paymentRecordUC.paymentRecordCheckLogRepo.LogFetchAttempt(paymentRecordCheckHTTP, delay)

				if err != nil {
					paymentRecordUC.logger.Error().Msgf("❌ Error fetching Payment Record with: %s", err)
				}

				if status == "PAID" || status == "UNPAID" {
					paymentRecordUC.logger.Info().Msgf("️🔄 Finalized: %s -> %s", id, status)
					_ = paymentRecordUC.paymentRecordRepo.PublishSuccessEvent(ctx, id)

					// ✅ Remove from in-memory tasks
					paymentRecordUC.tasks.Delete(id)

					// ✅ Remove from Redis persistence
					_ = paymentRecordUC.paymentRecordRepo.RemovePollingTask(ctx, id)
					return
				}

				_ = paymentRecordUC.paymentRecordRepo.SetNextRetry(ctx, id, delay)
				time.Sleep(delay)

				if delay < maxDelay {
					delay *= 2
				}
			}
		}
	}()
	paymentRecordUC.tasks.Store(id, true)
	_ = paymentRecordUC.paymentRecordRepo.PersistPollingTask(ctx, id)
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) BoostOtherTasks(id uuid.UUID) error {
	paymentRecordUC.tasks.Range(func(key, _ interface{}) bool {
		paymentID := key.(uuid.UUID)
		if paymentID != id {
			go paymentRecordUC.StartPolling(context.Background(), paymentID)
		}
		return true
	})
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) StartConsumer(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				paymentRecordUC.logger.Info().Msgf("⁉️Kafka consumer stopped")
				return
			default:
				paymentIDStr, err := paymentRecordUC.paymentRecordRepo.ReadKafkaMessage(ctx)
				if err != nil {
					paymentRecordUC.logger.Error().Msgf("❌ Kafka error: %s", err)
					continue
				}
				paymentID, err := uuid.Parse(paymentIDStr)
				if err != nil {
					paymentRecordUC.logger.Error().Msgf("❌ Invalid UUID: %s", err)
					continue
				}

				// Deduplication logic
				if _, loaded := seen.LoadOrStore(paymentID.String(), true); loaded {
					paymentRecordUC.logger.Info().Msgf("⁉️ Duplicate message ignored: %s", paymentID)
					continue
				}

				paymentRecordUC.logger.Info().Msgf("⚡ Boost triggered by: %s", paymentID)
				_ = paymentRecordUC.BoostOtherTasks(paymentID)
			}
		}
	}()
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error) {
	paymentRecordUC.logger.Info().Str("usecase", "Create").Msg("⚙️ Store payment records")
	tx, err := paymentRecordUC.db.Begin()
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to begin transaction")
		return nil, err
	}

	err = paymentRecordUC.paymentRecordRepo.Store(ctx, tx, &paymentRecord)
	if err != nil {
		tx.Rollback()
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to store payment records, rolling back")
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to commit transaction")
		return nil, err
	}

	paymentRecordUC.logger.Info().Str("payment_id", paymentRecord.ID.String()).Msg("✅ Payment records created")
	return &paymentRecord, nil
}

func (paymentRecordUC *paymentRecordUseCase) GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error) {
	paymentRecordUC.logger.Info().Str("usecase", "GetByID").Msg("⚙️ Fetching payment records by ID")
	return paymentRecordUC.paymentRecordRepo.FetchByID(ctx, id)
}

func (paymentRecordUC *paymentRecordUseCase) ListRunningTasks() []uuid.UUID {
	var ids []uuid.UUID
	paymentRecordUC.tasks.Range(func(key, value any) bool {
		if id, ok := key.(uuid.UUID); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func (paymentRecordUC *paymentRecordUseCase) RestorePollingTasks(ctx context.Context) error {
	ids, err := paymentRecordUC.paymentRecordRepo.RestorePollingTasks(ctx)
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to restore polling tasks from Redis")
		return err
	}

	for _, id := range ids {
		paymentRecordUC.logger.Info().Msgf("♻️ Restoring polling task: %s", id)
		_ = paymentRecordUC.StartPolling(ctx, id)
	}
	return nil
}
